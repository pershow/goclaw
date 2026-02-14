# 9router 406 错误最终解决方案

## 问题分析

### 错误信息
```
406 Not Acceptable {"message":"[iflow/kimi-k2.5] [406]: Unknown error (reset after 30s)"}
```

### 根本原因

9router 不支持某些 OpenAI 扩展参数，包括：
1. ❌ `reasoning_content` - Moonshot thinking 相关参数
2. ❌ `extra_body.reasoning` - 配置文件中的 reasoning 参数

---

## 问题演变

### 第一次修复（不完整）

**代码**：
```go
if p.router9Compatible {
    reqOpts = p.extraBodyOptions()  // ❌ 仍然会添加 extra_body
}
```

**问题**：虽然禁用了 `reasoning_content`，但仍然会添加配置文件中的 `extra_body`：
```json
"extra_body": {
  "reasoning": {
    "enabled": false
  }
}
```

这个 `reasoning` 参数导致 9router 返回 406 错误。

### 第二次修复（完整）

**代码**：
```go
if p.router9Compatible {
    reqOpts = []option.RequestOption{}  // ✅ 完全不添加任何额外参数
    logger.Debug("9router compatibility mode: disabled reasoning_content and extra_body")
}
```

**效果**：
- ✅ 不添加 `reasoning_content`
- ✅ 不添加 `extra_body` 中的任何参数
- ✅ 只发送基础的 OpenAI 兼容参数

---

## 9router 不支持的参数

### 1. reasoning_content

**来源**：Moonshot/Kimi API 的 thinking 功能

**示例**：
```json
{
  "messages": [
    {
      "role": "assistant",
      "reasoning_content": "思考过程...",
      "content": "回答内容..."
    }
  ]
}
```

**问题**：9router 不识别这个字段，导致 406 错误

### 2. extra_body.reasoning

**来源**：配置文件中的扩展参数

**示例**：
```json
{
  "extra_body": {
    "reasoning": {
      "enabled": false
    }
  }
}
```

**问题**：9router 不支持这个参数，即使设置为 `false` 也会导致 406 错误

### 3. 流式请求 (stream: true)

**依据**：9router 官方 [GitHub](https://github.com/decolua/9router) 与 iflow 执行器逻辑：转 iflow 时仅在 `stream=true` 时设置 `Accept: text/event-stream`，心流上游可能要求该头，否则易返回 406。

**处理**：在配置里对 9router 设置 `"streaming": true`（或 profile 不写时由顶层 `providers.9router.streaming` 决定）。不硬编码，由配置控制。

**配置建议**：base_url 使用 `http://127.0.0.1:20128/v1` 而非 localhost，避免 IPv6 解析问题（见 9router README OpenClaw 说明）。

---

## 完整的兼容性处理

### Chat 方法（非流式）

```go
// 9router 兼容模式：禁用 reasoning_content 和部分 extra_body 参数
var reqOpts []option.RequestOption
if p.router9Compatible {
    // 9router 兼容模式：不添加任何 extra_body 和 reasoning_content
    reqOpts = []option.RequestOption{}
} else {
    reqOpts = append(p.extraBodyOptions(), assistantReasoningOptions(messages)...)
}

completion, err := p.client.Chat.Completions.New(ctx, req, reqOpts...)
```

### ChatStream 方法（流式）

```go
// 9router 兼容模式：禁用 reasoning_content 和部分 extra_body 参数
var reqOpts []option.RequestOption
if p.router9Compatible {
    // 9router 兼容模式：不添加任何 extra_body 和 reasoning_content
    reqOpts = []option.RequestOption{}
    logger.Debug("9router compatibility mode: disabled reasoning_content and extra_body")
} else {
    reqOpts = append(p.extraBodyOptions(), assistantReasoningOptions(messages)...)
}

stream := p.client.Chat.Completions.NewStreaming(ctx, req, reqOpts...)
```

---

## Moonshot 调 kimi-k2.5 是怎么配的（可对照 9router）

Moonshot 和 9router 在代码里都走同一套 OpenAI 兼容 provider，只是 **base_url / 模型名 / 请求体是否带扩展参数** 不同。

### Moonshot（直连月之暗面，能调 kimi-k2.5）

- **模型配置**：`agents.defaults.model` 用 `kimi-k2.5` 或 `moonshot:kimi-k2.5`。  
  `determineProvider` 里：前缀 `kimi-` 或 `moonshot:` 会选 Moonshot，发给接口的 model 为 `kimi-k2.5`。
- **Provider 配置**：`providers.moonshot` 里配 `api_key`、`base_url`（不写则默认 `https://api.moonshot.cn/v1`）、可选 `streaming`、可选 `extra_body`（如关 thinking：`{"reasoning":{"type":"disabled"}}`）。
- **请求体**：会带 **extra_body** 和每条 assistant 的 **reasoning_content**（与 Moonshot thinking 要求一致）。

示例（仅示意）：

```json
"agents": { "defaults": { "model": "kimi-k2.5" } },
"providers": {
  "moonshot": {
    "api_key": "sk-moonshot-xxx",
    "base_url": "https://api.moonshot.cn/v1",
    "streaming": true,
    "extra_body": { "reasoning": { "type": "disabled" } }
  }
}
```

### 9router 调 kimi-k2.5（走心流 / iflow）

- **模型配置**：要用 9router 的路由格式，`agents.defaults.model` 配成 **`if/kimi-k2.5`**（或 `9router:if/kimi-k2.5`，后者会解析成 9router + model `if/kimi-k2.5`）。  
  `if/` 表示走 9router 的 iFlow，9router 再转给心流上的 kimi-k2.5。
- **Provider 配置**：`providers.9router` 里配 `api_key`、`base_url`（建议 `http://127.0.0.1:20128/v1`）、`streaming`、可选 `extra_body`。  
  和 Moonshot 一样都有这些字段，但**当前实现**里 9router 请求不会带 extra_body / reasoning_content（为兼容 9router/心流，避免 406）。
- **请求体差异**：发往 9router 的请求**不会**带 `extra_body` 和 `reasoning_content`（代码里 9router 兼容模式会清空这两类参数），其余（model、messages、stream、temperature、max_tokens、tools）和 Moonshot 一致。

示例（仅示意）：

```json
"agents": { "defaults": { "model": "if/kimi-k2.5" } },
"providers": {
  "9router": {
    "api_key": "sk-9router-xxx",
    "base_url": "http://127.0.0.1:20128/v1",
    "streaming": true
  }
}
```

若 9router 仍 406，可在 9router 侧开请求日志对比「Moonshot 直连成功请求」和「9router 请求」的 body 差异（见下文「仍 406 时怎么排查」）。

---

## 配置建议

### 方案 1：保留 extra_body（推荐）

保持配置文件不变，让代码自动处理：

```json
{
  "providers": {
    "9router": {
      "api_key": "sk_9router",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "streaming": true,
      "extra_body": {
        "reasoning": {
          "enabled": false
        }
      }
    }
  }
}
```

**优点**：
- ✅ 配置统一
- ✅ 代码自动处理兼容性
- ✅ 切换到其他 provider 时不需要修改配置

### 方案 2：移除 extra_body

如果只使用 9router，可以移除 `extra_body`：

```json
{
  "providers": {
    "9router": {
      "api_key": "sk_9router",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "streaming": true
    }
  }
}
```

**优点**：
- ✅ 配置更简洁
- ✅ 明确不使用扩展参数

---

## 验证修复

### 1. 重新编译

```bash
go build -o goclaw.exe .
```

### 2. 启动服务

```bash
./goclaw gateway run
```

### 3. 查看日志

应该看到：
```
INFO  Detected 9router proxy, enabling compatibility mode
DEBUG 9router compatibility mode: disabled reasoning_content and extra_body
```

### 4. 测试请求

发送测试消息，应该不再出现 406 错误。

---

## 修改的文件

### providers/openai.go

**修改位置 1**：Chat 方法（约 line 95-105）
```go
if p.router9Compatible {
    reqOpts = []option.RequestOption{}  // 完全不添加额外参数
} else {
    reqOpts = append(p.extraBodyOptions(), assistantReasoningOptions(messages)...)
}
```

**修改位置 2**：ChatStream 方法（约 line 430-437）
```go
if p.router9Compatible {
    reqOpts = []option.RequestOption{}  // 完全不添加额外参数
    logger.Debug("9router compatibility mode: disabled reasoning_content and extra_body")
} else {
    reqOpts = append(p.extraBodyOptions(), assistantReasoningOptions(messages)...)
}
```

---

## 技术细节

### OpenAI SDK 的参数传递

```go
// 基础参数（9router 支持）
req := openai.ChatCompletionNewParams{
    Model:    "if/kimi-k2.5",
    Messages: messages,
    Temperature: 0.6,
    MaxTokens: 8192,
}

// 扩展参数（9router 不支持）
reqOpts := []option.RequestOption{
    option.WithJSONSet("reasoning.enabled", false),           // ❌ 导致 406
    option.WithJSONSet("messages.0.reasoning_content", ""),  // ❌ 导致 406
}

// 发送请求
client.Chat.Completions.New(ctx, req, reqOpts...)
```

### 9router 兼容模式

```go
// 检测 9router
router9Compatible := strings.Contains(baseURL, ":20128")

// 根据兼容模式决定参数
if router9Compatible {
    reqOpts = []option.RequestOption{}  // 空参数列表
} else {
    reqOpts = append(extraBody, reasoningContent...)  // 完整参数
}
```

---

## 对比其他 Provider

| Provider | reasoning_content | extra_body.reasoning | 结果 |
|----------|-------------------|---------------------|------|
| Moonshot | ✅ 支持 | ✅ 支持 | 正常工作 |
| OpenAI | ❌ 忽略 | ❌ 忽略 | 正常工作（忽略未知参数） |
| 9router | ❌ 不支持 | ❌ 不支持 | 406 错误 |

---

## 当前请求里会传哪些参数

goclaw 发往 9router 的请求体由 OpenAI SDK 序列化，**一定会带的**只有：

| 参数 | 说明 |
|------|------|
| `model` | 模型名，如 if/kimi-k2.5 |
| `messages` | 消息列表 |
| `stream` | 流式时为 true |

**按配置/条件可能带的**：

| 参数 | 何时会传 |
|------|----------|
| `temperature` | 仅当配置里 `agents.defaults.temperature` > 0 |
| `max_tokens` | 仅当配置里 `max_tokens` > 0 |
| `tools` | 仅当本次调用有工具定义时 |

9router 兼容模式下**不会**通过 `reqOpts` 再塞入：`reasoning_content`、`extra_body` 里任意字段。

注意：SDK 在带 `tools` 时可能会自动加上 `tool_choice` 等字段，若上游不支持可能 406。若怀疑是 tools 导致，可临时关掉部分工具或先不用 tools 做一次请求对比。

### 9router 如何带 ENABLE_REQUEST_LOGS 启动

9router 官方说明：设环境变量 `ENABLE_REQUEST_LOGS=true` 后，请求/响应会写入 `logs/` 目录，便于对比 goclaw 与 OpenClaw 的实际请求体。

- **全局安装后命令行启动**：
  ```bash
  ENABLE_REQUEST_LOGS=true 9router
  ```
- **从源码 dev**（在 9router 项目根目录）：
  ```bash
  ENABLE_REQUEST_LOGS=true PORT=20128 NEXT_PUBLIC_BASE_URL=http://localhost:20128 npm run dev
  ```
- **从源码 production**：
  ```bash
  npm run build
  ENABLE_REQUEST_LOGS=true PORT=20128 HOSTNAME=0.0.0.0 NEXT_PUBLIC_BASE_URL=http://localhost:20128 npm run start
  ```
- **Docker**：在 `--env-file` 的 `.env` 里增加一行 `ENABLE_REQUEST_LOGS=true`，或在 `docker run` 时加 `-e ENABLE_REQUEST_LOGS=true`。
- **Windows PowerShell**（当前 shell 仅本次生效）：
  ```powershell
  $env:ENABLE_REQUEST_LOGS="true"; 9router
  ```

**日志目录**：请求会记到 9router **进程当前工作目录**下的 `logs/`。  
- 全局安装后直接运行 `9router`：日志在**你执行命令时所在目录**下的 `logs/`（例如在 `C:\Users\Administrator` 下运行则为 `C:\Users\Administrator\logs\`）。  
- 从源码 `npm run dev`/`start`：日志在 **9router 项目根目录**下的 `logs/`。  
- Docker：日志在容器内 `/app/logs/`，若未挂载该目录需进容器查看或挂载出来。  
详见 [9router README](https://github.com/decolua/9router) 的 Environment Variables 与 Troubleshooting。

### 仍 406 时怎么排查

1. **看 goclaw 调试日志**：把日志级别调到 debug，会打出 9router 请求的 model、messages 数、tools 数、是否带 temperature/max_tokens，便于确认“我们以为发了什么”。
2. **看 9router 实际收到的请求**：在 9router 所在环境设 `ENABLE_REQUEST_LOGS=true`，请求会记到 `logs/`，可看到完整请求体。启动方式见下文「9router 如何带 ENABLE_REQUEST_LOGS 启动」。
3. **和 openclaw 对比**：用同一模型在 openclaw 发一版成功请求，对比 9router 日志里 openclaw 的请求体与 goclaw 的请求体，看多/少了哪些字段。

4. **先试不传 tools**：在 `providers.9router` 下设置 `"tools_enabled": false`，请求体会不带 tools；若 406 消失，多半与 tools/tool_choice 有关。

5. **若 `tools_sent: 0` 仍 406**：说明与 tools 无关，可继续排查：消息条数/内容（如历史里是否有 assistant+tool_calls 等若心流不支持的格式）、model id（可试 9router 文档里的 `if/kimi-k2-thinking` 等）、9router 侧开 `ENABLE_REQUEST_LOGS=true` 看实际请求体、以及心流/9router 上游是否限流或 "reset after 30s" 类超时。

6. **试非流式**：在 `providers.9router` 下设置 `"streaming": false`（若启用 failover，profile 未填时回退到顶层 `providers.9router.streaming`）。重启后看日志应出现 **"Using non-streaming API"** 和 **"9router non-streaming request"**。若此时 406 消失，则与流式请求头/体有关。

7. **若非流式也 406**：说明与流式/非流式无关，问题在**请求体或请求头**（Go openai-go 发出的 JSON/头与 OpenClaw 的 Node/pi-ai 不同，9router 或心流只接受其中一种）。下一步：在 9router 所在环境设 `ENABLE_REQUEST_LOGS=true`，用 OpenClaw 成功请求一次、用 goclaw 失败请求一次，对比 9router 日志里两份请求的 body/header 差异，把多出或缺少的字段反馈给 9router/心流或据此改 goclaw。

### 为何 OpenClaw 可以、goclaw 却 406

- **请求栈不同**：OpenClaw 用 **Node + pi-ai** 调 9router（同一份 9router 配置、同一模型 `if/kimi-k2.5`），goclaw 用 **Go + openai-go**。两边发出的 HTTP 请求头、请求体字段/顺序可能不同，9router 或心流上游可能只接受其中一种形态。
- **流式、非流式都 406**：说明问题不在 stream 与否，而在**请求体或请求头本身**。必须用 9router 的 `ENABLE_REQUEST_LOGS=true` 拿到 goclaw 实际发出的请求，与 OpenClaw 成功请求对比，才能确定差异并做兼容（改 goclaw 或向 9router/心流提 issue）。

---

## 总结

### 问题根源

9router 对 OpenAI API 的兼容性比较严格，不支持 Moonshot 的扩展参数：
1. `reasoning_content` - Moonshot thinking 功能
2. `extra_body.reasoning` - 配置文件中的扩展参数

### 解决方案

在 9router 兼容模式下，完全不添加任何扩展参数：
```go
if p.router9Compatible {
    reqOpts = []option.RequestOption{}  // 空参数列表
}
```

### 关键修改

- **文件**：`providers/openai.go`
- **位置**：Chat 和 ChatStream 方法
- **修改**：从 `p.extraBodyOptions()` 改为 `[]option.RequestOption{}`

---

**更新日期**: 2026-02-14
**问题**: HTTP 406 Not Acceptable
**根本原因**: 9router 不支持 `extra_body.reasoning` 参数
**解决方案**: 9router 兼容模式下完全不添加扩展参数
