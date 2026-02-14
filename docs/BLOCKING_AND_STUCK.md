# 卡住/阻塞排查说明

本文说明可能导致 UI 或后端“卡住”的位置及缓解方式。

## 前端（UI 卡顿）

### 子 agent 进度刷新过于频繁
- **位置**：`ui/src/ui/app-tool-stream.ts` 中每收到一条子 agent 的 tool 事件就调用 `flushSubagentRunEntries()`，会触发 Lit 重绘。
- **现象**：子 agent 工具事件很多时，主会话页会频繁重绘，感觉“卡顿”。
- **已做**：增加 `scheduleSubagentFlush()` 节流（约 120ms），用定时器合并多次事件后再刷新一次 UI。

## 后端（Run 卡住不返回）

### 1. 工具执行阻塞（最常见）
- **位置**：`agent/orchestrator.go` 中 `executeToolCalls`：每个 tool 在独立 goroutine 里执行，结果写入 `resultsChan`；主流程 `wg.Wait()` 后 `for res := range resultsChan` 收齐结果。
- **原因**：若某个 **tool.Execute() 一直不返回**（例如同步 HTTP 无超时、死锁、无限循环），对应 goroutine 不会向 channel 发送结果，`wg.Wait()` 永远不返回，整个 **orchestrator.Run()** 会一直阻塞。
- **建议**：
  - 工具内所有外部调用（HTTP、DB、文件）应带 **context 超时** 或合理超时时间，并在 `ctx.Done()` 时尽快返回。
  - 不要在工具里无限等待或长时间占用锁。

### 2. LLM 调用阻塞 / 模型 API 断开后“卡死”
- **位置**：`orchestrator.Run()` 内通过 provider 调用 LLM（如 `streamAssistantResponse` → `ChatStream`）。
- **原因**：程序正常运行时，若**模型 API 端口/网络断开、上游无响应**等，当前 Run 的 context **没有超时**（gateway 触发的 Run 用的是进程级 ctx），provider 的 HTTP/流式调用会一直阻塞，**本次任务就停滞**，用户看到“一直转圈、卡死”。
- **建议**：
  - 为**单次 Run** 配置超时：在 `agents.defaults` 下设置 `run_timeout_seconds`（秒，0 表示不限制）。超时后 ctx 取消，LLM 调用返回错误，前端会收到 `phase: "error"`，用户可知是超时/网络问题而非静默卡死。示例：`"agents": { "defaults": { "run_timeout_seconds": 300 } }`（5 分钟）。
  - Provider 实现应尊重 `context.Context` 取消与超时；网关或 CLI 的“中止”应能取消该 context。

- **OpenClaw 对照**：
  - 配置：`agents.defaults.timeoutSeconds`（默认 **600** 秒），见 `src/agents/timeout.ts`（`resolveAgentTimeoutSeconds` / `resolveAgentTimeoutMs`）。`chat.send` 可传 `timeoutMs` 覆盖。
  - 机制：每次 `chat.send` 创建 **AbortController**，计算 `expiresAtMs = now + timeoutMs + graceMs` 存入 `chatAbortControllers`，并把 **abortSignal** 传给 agent 的 `dispatchInboundMessage`；agent/LLM 侧需在请求中带上该 signal，超时或用户点“停止”时 abort。
  - 超时执行：**server-maintenance.ts** 里与 dedupe 清理同周期的 **setInterval（约 60s）** 遍历 `chatAbortControllers`，若 `now > entry.expiresAtMs` 则调用 `abortChatRunById(..., stopReason: "timeout")`，对应该 run 的 controller.abort()，并广播 `chat` 事件 `state: "aborted"`。即由网关**周期性检查**过期 run 并主动 abort，而不是为每个 run 单独设 setTimeout。
  - goclaw 当前做法：用 **context.WithTimeout** 在 processMessageAsync 里为单次 Run 绑死线，到点 ctx 取消，无需维护 AbortController 表与定时扫描，实现更简单；效果类似（到点 Run 结束并报错）。

- **超时后的处理（已对齐）**：
  - **OpenClaw**：超时由 maintenance 调用 `abortChatRunById(..., stopReason: "timeout")` → **broadcast 一条 chat 事件**，`state: "aborted"`、`stopReason: "timeout"`。前端收到 **event: "chat"** 即可清空当前 run、停止转圈。
  - **goclaw**：Run 报错（含超时）时除 **agent** 事件（lifecycle `phase: "error"`）外，会**再发一条 chat 事件**：`publishRunErrorToBus` 发布 `OutboundMessage` 且 `ChatState: "error"`，gateway 广播 **event: "chat"**、`state: "error"`、`message.content` 为错误文案。前端可按与 "final"/"aborted" 同一套逻辑收尾（清空 chatRunId、停止转圈、展示错误）。

### 3. 入站队列写满导致 PublishInbound 阻塞
- **位置**：`bus/queue.go` 的 `PublishInbound` 使用 `select { case b.inbound <- msg; case <-ctx.Done() }`。若 `inbound` 已满且消费很慢，会阻塞直到有空间或 context 取消。
- **场景**：例如主会话正在跑一个很重的 Run，`processMessages` 单 goroutine 虽已用 `go processMessageAsync` 异步执行，但若某处同步等待或处理极慢，可能导致消费变慢、队列积压。
- **现状**：`handleInboundMessage` 仅做会话解析和 `go processMessageAsync`，本身不等待 Run 结束，因此通常不会因“等 Run”而堵住消费。若仍出现阻塞，需检查是否有其他同步路径占用 processMessages 循环。

### 4. Lane 串行与单 worker
- **位置**：`process/command_queue.go`：每个 lane（如 `session:main`、`subagent`）串行执行任务，一个任务不结束，同 lane 下一个不会开始。
- **现象**：同一 lane 内若有一个 Run 因上述 1 或 2 卡住，该 lane 会一直不消化新任务；其他 lane（如主会话 vs 子 agent）互不影响。
- **建议**：通过上下文取消（如用户点“停止”）结束卡住的 Run，或从根本解决 1/2 的阻塞来源。

### 5. 多 agent 同时调模型导致卡死
- **原因**：多个会话（多 agent 或主会话 + 子 agent）各自在不同 lane 并发执行，但**共享同一个 LLM Provider**。若上游 API（如 9router、部分代理）只支持单连接或并发能力很弱，多个同时的 Chat/ChatStream 会互相阻塞或拖死，表现为“都卡住、像卡死了”。
- **已做**：提供 **全局 LLM 并发限制**：在配置中增加 `providers.max_concurrent_calls`（默认 0 表示不限制）。设为 **1** 时，同一时刻只允许一个 LLM 请求在执行，多 agent 会排队，避免同时请求接口导致卡死。
- **配置示例**（`config.json`）：
  ```json
  "providers": {
    "max_concurrent_calls": 1,
    "9router": { ... }
  }
  ```
- **建议**：多 agent 或子 agent 场景下若出现整体卡死，优先将 `max_concurrent_calls` 设为 1；若上游支持少量并发，可设为 2 等。

### 5b. 同一会话内连续调模型导致 406（可配置间隔）
- **原因**：同一轮对话中，第一次 LLM 返回后立刻第二次请求（如工具调用后的下一轮），上游 9router/心流可能返回 406（reset after 30s）。
- **配置**：在 `agents.defaults` 下设置 **`model_request_interval_seconds`**（秒，0 表示不限制）。大于 0 时，同一 Run 内每次调用 LLM 前会等待，确保与上次调用至少间隔该秒数，再发请求。
- **示例**：`"model_request_interval_seconds": 35` 可配合「reset after 30s」使用，减少 406。
- **OpenClaw**：无此配置；OpenClaw 的 `humanDelay` 是块回复之间的拟人延迟，不是模型请求间隔。

### 6. 多会话流式事件串台（已修复）
- **原因**：此前同一 agent 的多个会话（或主会话 + 子 agent）共用一个 Orchestrator，即共用一个 `eventChan`。多个 Run 并发时，流式事件会混在一起被不同 Run 的消费者收到，导致前端看到“串流”、内容错会话。
- **已做**：每次 `executeAgentRun` 改为使用 `agent.CreateOrchestratorForRun(sessionKey)` 创建**本 Run 专属**的 Orchestrator，每个 Run 有独立事件通道，流式输出与 tool 事件按 sessionKey/runId 正确归属。

### 7. 调用模型时有没有“会话 id”保证不串流？
- **发给模型 API 的请求**：没有。每次 `Chat` / `ChatStream` 是独立 HTTP 请求，上游（OpenAI、9router 等）不维护会话；历史通过本次请求的 `messages` 数组传入，**请求体里不传会话 id**。
- **保证不串流**靠进程内两件事：
  1. **Run 级隔离**：每次执行用 `agent.CreateOrchestratorForRun(sessionKey)` 创建**本 Run 独占**的 Orchestrator，其内部有独立的 **eventChan**。流式 chunk 只在该 Orchestrator 内 `o.emit(EventMessageDelta)`，只有本 Run 的 goroutine 在消费该 channel，因此不会和别的 Run 混在一起。
  2. **事件带 runId / sessionKey**：所有发到总线的 agent 事件、chat 事件都带 **runId**（= 入站消息 ID，即前端 idempotencyKey）和 **sessionKey**。前端按 runId/sessionKey 过滤即可只显示当前会话、当前 run 的流。
- 因此“会话 id”在 goclaw 里是 **runId + sessionKey**，用于**事件归属**和**前端区分**，不传给模型 API。

## 为什么广播 final 后没有“调模型”的日志？Agent 流程有问题吗？

- **没有**。Agent 是**有入站才跑**的设计：
  - `processMessages` 一直在 `bus.ConsumeInbound` 上阻塞等待；
  - 只有用户通过 WebSocket 发新消息 → gateway 调 `PublishInbound` → 才会被消费 → `RouteInbound` → `processMessageAsync` → `executeAgentRun` → 才会调 LLM。
- 因此：**没有新消息时，不会调模型、也不会有新的 Run/LLM 日志**，这是预期行为。若用户没再发消息（或消息因 WebSocket 已断而没到服务端），后台“静默”是正常的。

## 有自动重连/“自动唤醒”吗？OpenClaw 程序逻辑是怎么做的？

- **“全局唤醒 agent”**：**没有**。OpenClaw 和 goclaw 一样，agent 只在有入站消息时跑，没有任何定时往 bus 推消息、或定时触发 Run 的“全局唤醒”逻辑。
- **Gateway 程序逻辑里的保活（OpenClaw 有，goclaw 暂无）**：
  - **OpenClaw**：在 **服务端** `src/gateway/server-maintenance.ts` 里有一个**定期 tick**：`setInterval` 每隔约 30s（`TICK_INTERVAL_MS`）向**所有 WebSocket 连接**执行 `broadcast("tick", { ts })` 和 `nodeSendToAllSubscribed("tick", payload)`。这是 **periodic keepalive**，不是唤醒 agent：
    - 服务端主动往下推数据，连接不会长时间无下行，有利于减少代理/中间件因“空闲”断连；
    - 客户端（`src/gateway/client.ts`）用收到的 tick 更新 `lastTick`，若超过 `tickIntervalMs * 2` 没收到 tick 就主动 `close(4000, "tick timeout")`，用于**客户端侧检测服务端无响应**。
  - **goclaw**：目前只有 WebSocket 的 Ping/Pong（服务端发 Ping、靠客户端 Pong 刷新读 deadline），**没有**类似 OpenClaw 的“服务端定期向所有连接广播 tick”的逻辑。若客户端或中间网络对 Ping 响应不好，更容易在长时间无业务数据时被读超时关掉。
- **前端自动重连**（与 OpenClaw 一致）：
  - **OpenClaw**：`ui/src/ui/gateway.ts` 里 WebSocket `close` 时调用 `scheduleReconnect()`，用 backoff（约 800ms 起，上限 15s）再次 `connect()`；重连成功后 `onHello` 里重置 `chatRunId`/`chatStream`、调 `loadAgents`/`loadChatHistory` 等（见 `app-gateway.ts`）。
  - **goclaw 的 UI**：沿用同一套 `GatewayBrowserClient`，同样在 `onClose` 里 `scheduleReconnect()`，因此**已有自动重连**。
- 若仍感觉“断了”：可调大服务端 `gateway.websocket.pong_timeout`；若要在程序逻辑上对齐 OpenClaw，可考虑在 goclaw gateway 里增加**定期向所有 WebSocket 连接广播 tick**（仅保活，不触发 agent），以减少“空闲断连”。

## 广播 final 后日志“停住”、感觉“突然断了”

- **现象**：日志最后一条是 `Broadcasting chat event to WebSocket`、`state: "final"`，之后没有新的请求或 Run 日志。
- **说明**：这一轮 Run 已正常结束，后端在把本次回复的 final 事件推给 WebSocket 后，对这一轮没有更多动作，**不再打日志是预期行为**。
- **“断了”的常见原因**：
  1. **WebSocket 空闲读超时**：服务端按 `ping_interval`（默认 30s）发 Ping，依赖客户端 Pong 来刷新读 deadline；若在 `pong_timeout`（默认 60s）内未收到 Pong 或任何数据，`ReadMessage` 会超时并关闭连接。关闭时日志会带 `reason: "read_timeout_idle"` 或 `"read_timeout"`。
  2. **客户端关闭**：关标签页、休眠、网络抖动等，服务端会收到关闭帧并打出 `reason: "client_close"`。
  3. **前端未处理 final**：收到 `state: "final"` 后若未刷新历史或更新 UI，用户可能误以为“断线”或“卡住”。
- **建议**：
  - 若需长时间只看不操作，可在配置中调大 WebSocket 超时，例如：`gateway.websocket.pong_timeout` 改为 `120s` 或更长（需与前端/代理超时协调）。
  - 排查时看关闭瞬间是否出现 `WebSocket connection closed` 及 `reason` 字段，便于区分超时与客户端关闭。

### LLM 返回空回复（content_length: 0）导致前端一直转圈（已修复）

- **现象**：日志里 Run 正常结束（`Orchestrator Run End`、`Execution completed`），但 `content_length: 0, tool_calls_count: 0`，之后只有 outbound 心跳，前端一直转圈、没有新消息。
- **原因**：此前当 LLM 返回**空文本**时，`publishToBus` 会跳过发布（不推 chat 事件），前端收不到 `state: "final"`，无法结束当前 run。
- **已做**：Run 正常结束且最后一条为 assistant 但内容为空时，改为调用 `publishRunFinalToBus` 发送一条 **state: "final"**、内容可为空的 chat 事件，前端收到后即可清空 run、停止转圈。

## 快速排查顺序

1. **UI 卡顿**：看是否在子 agent 进度更新时发生；已通过节流缓解。
2. **主会话/子 agent 一直“转圈”不结束**：先看日志是否卡在某一 tool 或某次 LLM 调用；再检查该 tool 或 provider 是否未尊重 context/超时。若为**模型 API 断开/不可达**导致静默卡住，可配置 `agents.defaults.run_timeout_seconds`（如 300），超时后 Run 会返回错误而非一直卡死。
3. **新消息发不出去/收不到**：看 `processMessages` 是否被阻塞（如某次 RouteInbound 里有同步长耗时）、或 bus inbound 是否写满。
4. **广播 final 后“突然断了”**：确认是否为 WebSocket 空闲超时（看 `reason: read_timeout_idle`）或客户端关闭（`reason: client_close`）；必要时调大 `gateway.websocket.pong_timeout` 或检查前端对 final 的处理与重连。
