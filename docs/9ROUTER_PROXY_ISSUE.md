# 9router 代理 406 错误问题分析与解决方案

## 问题描述

用户在使用 goclaw 时遇到 HTTP 406 错误：

```
ERROR agent/orchestrator.go:356 LLM streaming call failed
error: stream error: POST "http://localhost:20128/v1/chat/completions": 406 Not Acceptable
{"message":"[iflow/kimi-k2.5] [406]: Unknown error (reset after 30s)"}
```

## 问题分析

### 1. 错误原因

HTTP 406 Not Acceptable 错误通常表示：
- 服务器无法提供客户端请求的内容格式
- 请求头中的 Accept 或 Content-Type 不被服务器接受
- 代理服务器（9router）可能不支持某些请求参数

### 2. 9router 代理

- 9router 是一个本地 API 代理工具，监听在 `localhost:20128`
- 用户提到 "openclaw就支持9router这么配置"，说明 openclaw 对 9router 有特殊处理
- goclaw 当前可能没有针对 9router 的兼容性处理

### 3. 可能的不兼容点

根据代码分析，goclaw 在发送请求时可能包含了 9router 不支持的参数：

1. **reasoning_content 字段**（`openai.go:302`）
   ```go
   // 始终设置 reasoning_content，无则传 ""，满足 Moonshot thinking 对 tool call 消息的要求
   opts = append(opts, option.WithJSONSet(fmt.Sprintf("messages.%d.reasoning_content", i), reasoning))
   ```

2. **extra_body 参数**（`openai.go:410`）
   ```go
   reqOpts := append(p.extraBodyOptions(), assistantReasoningOptions(messages)...)
   ```

3. **流式请求的特殊处理**
   - 9router 可能对流式请求有不同的处理方式

## 解决方案

### 方案 1：检测 9router 并禁用不兼容特性

修改 `providers/openai.go`，添加 9router 检测：

```go
// is9Router 检测是否使用 9router 代理
func (p *OpenAIProvider) is9Router() bool {
    return strings.Contains(p.baseURL, "localhost:20128") ||
           strings.Contains(p.baseURL, "127.0.0.1:20128")
}

// ChatStream 方法中添加检测
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, callback StreamCallback, options ...ChatOption) error {
    // ... 现有代码 ...

    reqOpts := []option.RequestOption{}

    // 9router 不支持 reasoning_content 和某些 extra_body 参数
    if !p.is9Router() {
        reqOpts = append(p.extraBodyOptions(), assistantReasoningOptions(messages)...)
    }

    stream := p.client.Chat.Completions.NewStreaming(ctx, req, reqOpts...)
    // ... 其余代码 ...
}
```

### 方案 2：配置中添加 9router 兼容模式

在 `config/schema.go` 中添加配置选项：

```go
type ProviderConfig struct {
    APIKey      string                 `json:"api_key"`
    BaseURL     string                 `json:"base_url"`
    Timeout     int                    `json:"timeout"`
    MaxRetries  int                    `json:"max_retries"`
    ExtraBody   map[string]interface{} `json:"extra_body"`

    // 新增：9router 兼容模式
    Router9Compatible bool `json:"router9_compatible"`
}
```

配置文件示例：

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-xxx",
      "base_url": "http://localhost:20128/v1",
      "router9_compatible": true,
      "timeout": 600,
      "max_retries": 3
    }
  }
}
```

### 方案 3：使用环境变量覆盖

如果 9router 通过系统代理工作，可以设置环境变量：

```bash
# Windows
set HTTP_PROXY=http://localhost:20128
set HTTPS_PROXY=http://localhost:20128

# Linux/Mac
export HTTP_PROXY=http://localhost:20128
export HTTPS_PROXY=http://localhost:20128
```

但这种方式可能仍然会遇到 406 错误，因为问题在于请求参数，而不是代理本身。

### 方案 4：临时禁用 reasoning_content（快速修复）

修改 `providers/openai.go:291-305`：

```go
func assistantReasoningOptions(messages []Message) []option.RequestOption {
    // 临时禁用 reasoning_content，避免 9router 406 错误
    return nil

    /* 原代码
    if len(messages) == 0 {
        return nil
    }
    opts := make([]option.RequestOption, 0)
    for i, msg := range messages {
        if msg.Role != "assistant" {
            continue
        }
        reasoning := strings.TrimSpace(msg.ReasoningContent)
        opts = append(opts, option.WithJSONSet(fmt.Sprintf("messages.%d.reasoning_content", i), reasoning))
    }
    return opts
    */
}
```

## 推荐方案

**推荐使用方案 1 + 方案 2 的组合**：

1. 自动检测 9router（通过 base_url）
2. 提供配置选项让用户手动启用兼容模式
3. 在兼容模式下禁用不兼容的特性

## 实施步骤

1. 修改 `providers/openai.go` 添加 9router 检测
2. 修改 `config/schema.go` 添加配置选项
3. 更新配置文件示例
4. 添加文档说明 9router 使用方法
5. 测试验证

## OpenClaw 的处理方式

需要查看 OpenClaw 的源码来了解它是如何处理 9router 的：
- 是否有特殊的请求头处理
- 是否禁用了某些参数
- 是否有重试机制

## 后续优化

1. 添加更详细的错误日志，显示实际发送的请求参数
2. 添加请求/响应拦截器，便于调试
3. 支持更多本地代理工具（如 one-api, new-api 等）
4. 添加集成测试，验证各种代理的兼容性

## 参考资料

- 9router 文档：https://github.com/9router/9router
- OpenAI API 规范：https://platform.openai.com/docs/api-reference
- HTTP 406 错误说明：https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/406
