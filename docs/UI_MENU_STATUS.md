# GoClaw 控制台 UI 菜单与功能状态

本文说明 Web 控制台（`ui/`）侧边栏菜单及各页面对 goclaw 后端的实现情况。

## 品牌与 Resources

- **品牌**：顶部显示 **GoClaw** / **Control UI**（已从 OpenClaw 文案改为 goclaw 专用）。
- **Resources 菜单**：
  - **Docs**：链接到 [GoClaw README](https://github.com/smallnest/goclaw#readme)。
  - **GitHub**：链接到 [github.com/smallnest/goclaw](https://github.com/smallnest/goclaw)。

## 侧栏分组与 Tab

| 分组 | Tab | 说明 |
|------|-----|------|
| Chat | chat | 与 agent 对话，已对接 `chat.history` / `chat.send` / `agent` / `agent.wait`，子任务进度已展示。 |
| Control | overview | 网关状态、连接配置；连接/刷新已对接。 |
| Control | channels | 展示 channel 状态卡片；后端有 `channels.status`，UI 为 OpenClaw 多 channel 设计，goclaw 返回的 channel 类型可能较少。 |
| Control | instances | Presence 列表；已对接 `system-presence`。 |
| Control | sessions | 会话列表与操作；已对接 `sessions.list` / `sessions.patch` / `sessions.delete`。 |
| Control | usage | 使用量/成本；已对接 `sessions.usage` / `usage.cost` 等。 |
| Control | cron | 定时任务；已对接 `cron.*`。 |
| Agent | agents | Agent 列表、身份、工作区文件、配置片段；已对接 `agents.list` / `agent.identity.get` / `agents.files.*` 等。 |
| Agent | skills | 技能列表与安装；已对接 `skills.status` / `skills.install` 等。 |
| Agent | nodes | 节点、设备配对、执行审批；已对接 `node.list` / `device.pair.*` / `exec.approvals.*`。 |
| Settings | config | 配置编辑；已对接 `config.get` / `config.set` / `config.apply`。 |
| Settings | debug | 调试与手动 RPC；已对接 `health` / `status` 等。 |
| Settings | logs | 日志；已对接 `logs.get` / `logs.tail`。 |

## 可能未完全对齐或受限的部分

1. **Channels**：UI 为 WhatsApp / Telegram / Discord / Slack / Signal / iMessage / Nostr 等多 channel 设计；goclaw 的 `channels.status` 返回结构若与 OpenClaw 不一致，部分卡片可能显示不全或需适配。
2. **Usage**：依赖后端 `sessions.usage`、`sessions.usage.timeseries`、`sessions.usage.logs`、`usage.cost` 等；若 goclaw 未实现或返回格式不同，该页会无数据或报错。
3. **Nodes / Devices**：设备配对、执行审批等依赖 `device.pair.*`、`exec.approvals.*`；若后端未启用或未实现，对应区块为空或仅显示“无数据”。
4. **Overview 中的文档链接**：overview 页内部分链接仍指向 `docs.openclaw.ai`（Tailscale、Control UI 等），仅作参考；goclaw 无独立文档站时以 README 与本仓库 `docs/` 为准。

以上为当前菜单与功能的实现与限制概览；后端 RPC 与前端调用的对应关系见 `gateway/handler.go` 中的 `Register` 列表。
