# OpenClaw 前端 Agent 界面参考（数据获取与子 agent 处理）

goclaw 前端可与 OpenClaw 对齐时参考本文：OpenClaw 的 agent/chat 界面如何获取数据、如何展示，以及子 agent 完成后如何在前端体现。

## 一、数据获取与展示

### 1. 会话列表
- **入口**：`ui/src/ui/controllers/sessions.ts` 的 `loadSessions(state, overrides?)`
- **请求**：`state.client.request("sessions.list", params)`  
  - 常用 params：`includeGlobal`, `includeUnknown`, `activeMinutes`, `limit`
- **结果**：写入 `state.sessionsResult`（`SessionsListResult`），供会话选择器、当前会话展示等使用。

### 2. 当前会话的聊天历史（消息列表）
- **入口**：`ui/src/ui/controllers/chat.ts` 的 `loadChatHistory(state)`
- **请求**：`state.client.request("chat.history", { sessionKey: state.sessionKey, limit: 200 })`
- **结果**：`state.chatMessages` = 消息数组，`state.chatThinkingLevel` = 思考级别；页面用 `chatMessages` 渲染历史消息。

### 3. 发送消息
- **入口**：`controllers/chat.ts` 的 `sendChatMessage(state, message, attachments?)`
- **请求**：`state.client.request("chat.send", { sessionKey, message, deliver: false, idempotencyKey: runId, attachments })`
- **本地**：先往 `chatMessages` 追加用户消息，设 `chatRunId`、`chatStream`、`chatStreamStartedAt`，再发 RPC。

### 4. 实时数据（WebSocket 事件）
- **网关分发**：`app-gateway.ts` 的 `handleGatewayEventUnsafe` 根据 `evt.event` 分支：
  - **`agent`**：调用 `handleAgentEvent(host, evt.payload)`（见 `app-tool-stream.ts`）
  - **`chat`**：调用 `handleChatEvent(host, payload)`（见 `controllers/chat.ts`）

- **Agent 事件（tool 流）**
  - `app-tool-stream.handleAgentEvent` 只处理 `payload.stream === "tool"`。
  - 要求：`payload.sessionKey === host.sessionKey`（否则直接 return）；且 `host.chatRunId` 存在且 `payload.runId === host.chatRunId`。
  - 效果：**只展示当前 tab 的 sessionKey 对应的那一次 run 的 tool 流**；子 agent 的 sessionKey 不同，其 agent 事件会被丢弃，主会话页不会显示子 agent 的 tool 进度。

- **Chat 事件（流式/结束/错误）**
  - `handleChatEvent` 先要求 `payload.sessionKey === state.sessionKey`，否则 return。
  - `state === "delta"`：用 `extractText(payload.message)` 更新 `chatStream`（流式正文）。
  - `state === "final"` / `"aborted"` / `"error"`：清空 `chatStream`、`chatRunId` 等；若是 `"final"` 还会在后续触发 `loadChatHistory()`（见下）。

## 二、子 agent 处理完了怎么处理

### 后端（OpenClaw）
- 子 agent 完成后，会把结果作为一条**主会话消息**写入主会话 transcript，并向该主会话推送 **chat 事件**：
  - 例如 `server-methods/chat.ts` 里 `transcript.append` 类逻辑：向某 sessionKey 追加一条 assistant 消息后，`broadcast("chat", chatPayload)` 且 `nodeSendToSession(sessionKey, "chat", chatPayload)`，payload 含 `state: "final"`, `sessionKey`, `message` 等。
- 这样主会话的 sessionKey 会收到一条 `event: "chat"`, `state: "final"` 的事件，message 即为子 agent 的最终回复（或摘要）。

### 前端（OpenClaw）
- **controllers/chat.ts** 中特意处理了「别的 run 的 final」：
  ```ts
  // Final from another run (e.g. sub-agent announce): refresh history to show new message.
  // See https://github.com/openclaw/openclaw/issues/1909
  if (payload.runId && state.chatRunId && payload.runId !== state.chatRunId) {
    if (payload.state === "final") {
      return "final";
    }
    return null;
  }
  ```
- 即：当收到的是**当前 sessionKey** 下、但 **runId ≠ 当前 chatRunId** 的 **final**（例如子 agent 完成时往主会话 announce 的 runId），仍返回 `"final"`。
- **app-gateway** 里对 `handleChatEvent` 返回 `"final"` 时会：
  - `resetToolStream`
  - `flushChatQueueForEvent`
  - 若该 runId 在 `refreshSessionsAfterChat` 里，会再 `loadSessions(...)`
  - **一定会** `loadChatHistory(host)` 
- 因此：**子 agent 完成后，主会话会重新拉 chat.history，子 agent 的结果作为主会话里新的一条消息被展示**；没有单独“子 agent 完成”的区块，而是和主会话消息混在一起。

## 三、与 goclaw 的对应关系

| 能力           | OpenClaw 做法                         | goclaw 可对齐点 |
|----------------|----------------------------------------|-----------------|
| 会话列表       | `sessions.list` → sessionsResult       | 已用 sessions.list，结构对齐即可 |
| 历史消息       | `chat.history` → chatMessages          | 已用 chat.history |
| 发送消息       | `chat.send` + 本地追加 + chatRunId     | 已用 chat.send |
| 当前 run 的 tool 流 | agent 事件，sessionKey + runId 一致才显示 | 已按 sessionKey/runId 过滤 |
| 子 agent 进度  | 不展示（子 agent 的 agent 事件被丢弃） | goclaw 已做：主会话下展示 subagentRunEntries |
| 子 agent 完成  | chat final（runId≠当前）→ 返回 "final" → loadChatHistory | goclaw 后端若把子 agent 结果发到主会话的 chat final，前端同样用 handleChatEvent 返回 "final" + loadChatHistory 即可 |

若 goclaw 后端在子 agent 完成时已通过 `sendToSession(RequesterSessionKey, ...)` 或等价方式向主会话发 chat 事件（state final + message），则前端只需保证与 OpenClaw 一致：**sessionKey 匹配时，对 runId 不同的 final 仍返回 "final" 并触发 loadChatHistory**，主会话列表里就会自然出现子 agent 的那条结果消息。

### 后端子 agent 完成逻辑（已与 OpenClaw 对齐）
- **不合并 session**：不把子会话的 transcript 合并进主会话；只读取子会话**最后一条 assistant 回复**（`readLatestAssistantReply`）作为 Findings。
- **宣告**：将 Findings + 状态 + 说明拼成 trigger message，通过 `sendToSession(RequesterSessionKey, triggerMessage)` 发给主会话，主 agent 据此总结并回复用户。
- **删除子会话**：当 spawn 时 `cleanup=delete` 时，宣告成功后调用 `sessionMgr.Delete(ChildSessionKey)` 删除子会话（含 transcript）；`cleanup=keep` 时不删。

### 启动时未删除/未收尾子 agent 的恢复（已与 OpenClaw 对齐）
- **OpenClaw**：`initSubagentRegistry()` → `restoreSubagentRunsOnce()` 从磁盘加载注册表，对每条 run 调用 `resumeSubagentRun(runId)`：若已结束（有 endedAt）则补跑宣告与 cleanup；若未结束则 `waitForSubagentCompletion`（agent.wait）超时或返回后标记 outcome 并做宣告与 cleanup。
- **goclaw**：在 `setupSubagentSupport` 中，`LoadFromDisk` 与 `SetOnRunComplete` 之后调用 **`RecoverAfterRestart()`**：
  - **已结束但未清理**（进程异常退出前子 agent 已跑完但未执行 announce/cleanup）：对每条调用 `onRunComplete` → `handleSubagentCompletion`，补宣告并按 `cleanup=delete` 删子会话。
  - **未结束**（进程异常退出时子 agent 仍在跑）：将记录标记为 `EndedAt=now`、`Outcome={ status: "error", error: "interrupted by restart" }`，再调用 `onRunComplete`，同样走宣告与可选删会话。
