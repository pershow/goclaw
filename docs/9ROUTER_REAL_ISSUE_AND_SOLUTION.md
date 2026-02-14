# 9router 配置问题真正原因和解决方案

## 问题分析

### ❌ 之前的错误理解

我之前错误地认为 9router 需要真实的 Moonshot API Key，这是**不正确的**。

### ✅ 正确理解

9router 是一个**智能路由工具**，它：
1. **自己管理后端 API Key** - 在 9router 的配置中设置
2. **提供统一的认证** - 客户端使用 `sk_9router` 即可
3. **自动路由请求** - 根据模型名称路由到不同的后端

---

## 真正的问题

### 问题 1：模型名称不匹配

**错误配置**：
```json
{
  "agents": {
    "defaults": {
      "model": "kimi-k2.5"  // ❌ 错误
    }
  }
}
```

**正确配置**：
```json
{
  "agents": {
    "defaults": {
      "model": "if/kimi-k2.5"  // ✅ 正确
    }
  }
}
```

**原因**：
- 9router 返回的模型 ID 是 `if/kimi-k2.5`
- `if` 是 provider 前缀（infoflow）
- 必须使用完整的模型 ID

### 问题 2：API Key 理解错误

**我之前的错误理解**：
```json
{
  "9router": {
    "api_key": "sk-real-moonshot-key"  // ❌ 错误理解
  }
}
```

**正确配置**：
```json
{
  "9router": {
    "api_key": "sk_9router"  // ✅ 正确
  }
}
```

**原因**：
- 9router 自己管理后端 API Key
- 客户端只需要使用 `sk_9router` 作为认证令牌
- 真实的 API Key 配置在 9router 服务端

---

## 9router 的工作原理

### 架构图

```
goclaw
  ↓ (使用 sk_9router)
9router (localhost:20128)
  ↓ (使用真实 API Key，在 9router 配置中)
后端 API (Moonshot/OpenAI/Anthropic 等)
  ↓
返回响应
```

### 模型路由

9router 根据模型前缀路由到不同的后端：

| 前缀 | 后端 | 示例模型 |
|------|------|----------|
| `if/` | Infoflow (如流) | `if/kimi-k2.5` |
| `cx/` | Claude X | `cx/gpt-5.3-codex` |
| `ag/` | Anthropic/Google | `ag/claude-opus-4-6-thinking` |
| `gh/` | GitHub Models | `gh/gpt-5` |
| `kr/` | Korea Region | `kr/claude-sonnet-4.5` |

### 可用模型列表

从 9router 返回的模型：

**Infoflow (if/) - 如流**：
- `if/kimi-k2.5` ⭐ (你要用的)
- `if/kimi-k2`
- `if/kimi-k2-thinking`
- `if/deepseek-r1`
- `if/deepseek-v3.2-chat`
- `if/qwen3-coder-plus`
- `if/minimax-m2.1`
- `if/glm-4.7`

**Claude X (cx/)**：
- `cx/gpt-5.3-codex`
- `cx/gpt-5.2-codex`
- `cx/gpt-5.1-codex`

**Anthropic/Google (ag/)**：
- `ag/claude-opus-4-6-thinking`
- `ag/claude-sonnet-4-5`
- `ag/gemini-3-pro-high`

**GitHub Models (gh/)**：
- `gh/gpt-5.1`
- `gh/claude-opus-4.1`
- `gh/gemini-3-pro`

---

## 正确的配置

### 完整配置文件

```json
{
  "agents": {
    "defaults": {
      "model": "if/kimi-k2.5",
      "max_iterations": 15,
      "temperature": 1,
      "max_tokens": 8192,
      "subagents": {
        "max_concurrent": 8,
        "archive_after_minutes": 60,
        "model": "if/kimi-k2.5",
        "timeout_seconds": 300
      }
    }
  },
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
    },
    "failover": {
      "enabled": true,
      "strategy": "round_robin"
    },
    "profiles": [
      {
        "name": "9router-primary",
        "provider": "9router",
        "api_key": "sk_9router",
        "base_url": "http://localhost:20128/v1",
        "priority": 1
      }
    ]
  }
}
```

### 关键配置项

1. **模型名称**：`if/kimi-k2.5`（必须包含前缀）
2. **API Key**：`sk_9router`（9router 的统一认证令牌）
3. **Base URL**：`http://localhost:20128/v1`（9router 地址）

---

## 验证配置

### 1. 测试 9router 连接

```bash
curl http://localhost:20128/v1/models \
  -H "Authorization: Bearer sk_9router"
```

应该返回模型列表（已验证 ✅）

### 2. 测试聊天请求

```bash
curl http://localhost:20128/v1/chat/completions \
  -H "Authorization: Bearer sk_9router" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "if/kimi-k2.5",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

### 3. 启动 goclaw

```bash
./goclaw gateway run
```

应该看到：
```
INFO  LLM provider resolved  provider=9router model=if/kimi-k2.5
INFO  Detected 9router proxy, enabling compatibility mode
```

---

## 错误对比

### 之前的错误

| 配置项 | 错误值 | 问题 |
|--------|--------|------|
| model | `kimi-k2.5` | ❌ 缺少前缀 |
| api_key | `sk-real-key` | ❌ 不需要真实 key |

### 现在的正确配置

| 配置项 | 正确值 | 说明 |
|--------|--------|------|
| model | `if/kimi-k2.5` | ✅ 包含前缀 |
| api_key | `sk_9router` | ✅ 9router 令牌 |

---

## 9router 的优势

### 1. 统一认证
- 客户端只需要 `sk_9router`
- 不需要管理多个 API Key

### 2. 智能路由
- 根据模型前缀自动路由
- 支持多个后端提供商

### 3. 负载均衡
- 自动分配请求
- 提高可用性

### 4. 成本优化
- 统一管理配额
- 优化 API 调用

---

## 常见问题

### Q1: 为什么不能用 `kimi-k2.5`？

**A**: 9router 需要完整的模型 ID（包含前缀）来识别路由目标。`if/kimi-k2.5` 表示使用 Infoflow 后端的 kimi-k2.5 模型。

### Q2: `sk_9router` 是什么？

**A**: 这是 9router 提供的统一认证令牌。客户端使用这个令牌，9router 会在后端使用真实的 API Key。

### Q3: 如何查看可用模型？

**A**:
```bash
curl http://localhost:20128/v1/models \
  -H "Authorization: Bearer sk_9router" | jq '.data[].id'
```

### Q4: 如何切换模型？

**A**: 修改配置文件中的 `model` 字段为其他可用模型，例如：
- `if/deepseek-r1`
- `ag/claude-opus-4-6-thinking`
- `gh/gpt-5.1`

---

## 总结

### 问题根源

1. ❌ 模型名称缺少前缀（`kimi-k2.5` → `if/kimi-k2.5`）
2. ❌ 误解了 9router 的认证机制

### 解决方案

1. ✅ 使用完整的模型 ID：`if/kimi-k2.5`
2. ✅ 使用 9router 令牌：`sk_9router`
3. ✅ 保持 base_url：`http://localhost:20128/v1`

### 关键理解

- 9router 是**智能路由工具**，不是简单的代理
- 9router **自己管理后端 API Key**
- 客户端只需要使用 **`sk_9router`** 令牌
- 模型名称必须包含**前缀**（如 `if/`、`ag/`、`gh/`）

---

**更新日期**: 2026-02-14
**问题**: "No credentials for provider: openai"
**真正原因**: 模型名称缺少前缀
**解决方案**: 使用 `if/kimi-k2.5` 而不是 `kimi-k2.5`
