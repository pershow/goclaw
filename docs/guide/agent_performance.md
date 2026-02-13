# Agent 执行与性能

## 卡死与无限循环（已防护）

- **无限循环**：若模型每轮都返回 tool_calls，内层循环会一直执行。现已按配置的 `agents.defaults.max_iterations`（默认 15）做**最大轮数限制**，超过即退出并返回错误，避免死循环。
- **卡死无响应**：事件通过 `eventChan` 发给消费者，若消费者阻塞（例如下游总线或网关很慢），orchestrator 在 `emit` 时可能一直阻塞。现已改为**非阻塞 emit**：通道满时丢弃该条事件并打 Debug 日志，orchestrator 不再被拖住；流式中间可能少几条 delta，最终完整回复仍会通过 `publishToBus` 发出。
- **超时/取消**：每轮开始时检查 `ctx.Err()`，若调用方传入带超时或可取消的 context，超时或取消后会立即退出循环并返回。建议对单次 agent 调用使用 `context.WithTimeout`（例如 120s），以便长时间无响应时能自动结束。

## 执行流程简述

1. **入站** → 解析 session、加载历史 → 构造 `allMessages`
2. **Orchestrator.Run** → 循环：构建上下文 → 调 LLM（流式/非流式）→ 若有 tool_calls 则**顺序执行**工具 → 把结果加入 state → 再调 LLM，直到无 tool_calls
3. **事件** → 每轮/每条消息/每个流式片段/每次工具执行都会向 `eventChan` 发事件；manager 中 goroutine 消费事件并调用 `publishStreamDelta` → `bus.PublishOutbound`
4. **出站** → 总线 fanout 到 gateway 等订阅者，前端收到流式或最终消息

## 已修复的瓶颈（导致“执行不顺畅”）

- **总线与日志热路径**：`PublishOutbound` 之前对每条出站消息打两条 Info 日志，流式时每条 delta 都会触发，日志 I/O 成为瓶颈并拖慢事件消费者，进而让 orchestrator 在 `emit` 时阻塞。已改为 Debug，仅调试时可见。
- **Orchestrator 热路径日志**：流式开始/结束、每轮 tool 开始/成功等由 Info 改为 Debug，减少日志写入。
- **事件通道缓冲**：`eventChan` 由 100 提升为 512，减轻流式输出时瞬时积压导致的阻塞。

## 仍可能影响顺畅度的因素

| 环节 | 说明 |
|------|------|
| **工具顺序执行** | 同一轮多个 tool_calls 串行执行，耗时会叠加。后续可考虑对无依赖的 tool 并行执行。 |
| **上下文构建** | 每轮 LLM 调用前会：限制历史轮次、截断 tool 结果、构建 system prompt（含 bootstrap 文件、记忆、技能列表）。bootstrap/记忆为读文件，通常较快。 |
| **会话历史加载** | `sess.GetHistory(-1)` 加载全部历史，会话很长时会有一定开销。 |
| **日志级别** | 生产环境建议使用 `info`；排查卡顿时可开 `debug` 观察详细事件与总线日志。 |

## 建议

- 若仍感觉“模型很快但整体卡”：看网关/前端是否处理流式过慢，或是否有其他订阅者拖慢总线消费。
- 若单轮很慢：看是否为工具执行耗时（如 `run_shell`、`smart_search`、浏览器）或 LLM 本身延迟。
