# 9router API Key 配置说明

## 问题说明

### 错误信息
```
"No credentials for provider: openai"
```

### 原因分析

9router 是一个**本地代理工具**，它的工作原理是：

```
goclaw → 9router (localhost:20128) → 真实的 API 提供商 (Moonshot/OpenAI)
```

因此，9router 需要**真实的 API Key** 来调用后端服务，而不是 `sk_9router` 这个占位符。

---

## 正确配置

### ❌ 错误配置

```json
{
  "providers": {
    "9router": {
      "api_key": "sk_9router",  // ❌ 错误：这只是占位符
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

### ✅ 正确配置

```json
{
  "providers": {
    "9router": {
      "api_key": "sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV",  // ✅ 正确：真实的 Moonshot API Key
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

---

## 9router 的工作原理

### 1. 请求流程

```
用户请求
    ↓
goclaw (使用真实 API Key)
    ↓
9router 代理 (localhost:20128)
    ↓
Moonshot API (api.moonshot.cn)
    ↓
返回响应
```

### 2. API Key 的作用

- **goclaw 配置中的 API Key**: 传递给 9router
- **9router**: 使用这个 API Key 调用真实的 Moonshot API
- **Moonshot API**: 验证 API Key 并返回结果

### 3. 为什么需要真实 API Key

9router 本身**不提供 AI 服务**，它只是一个**代理/路由工具**，用于：
- 负载均衡
- 请求转发
- 统一接口
- 监控和日志

所以它必须使用真实的 API Key 来调用后端服务。

---

## 配置示例

### 场景 1：使用 Moonshot API

```json
{
  "providers": {
    "9router": {
      "api_key": "sk-your-real-moonshot-key",
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

### 场景 2：使用 OpenAI API

```json
{
  "providers": {
    "9router": {
      "api_key": "sk-your-real-openai-key",
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

### 场景 3：使用故障转移

```json
{
  "providers": {
    "failover": {
      "enabled": true,
      "strategy": "round_robin"
    },
    "profiles": [
      {
        "name": "9router-primary",
        "provider": "9router",
        "api_key": "sk-your-real-moonshot-key",
        "base_url": "http://localhost:20128/v1",
        "priority": 1
      },
      {
        "name": "moonshot-backup",
        "provider": "moonshot",
        "api_key": "sk-your-real-moonshot-key",
        "base_url": "https://api.moonshot.cn/v1",
        "priority": 2
      }
    ]
  }
}
```

---

## 常见误解

### ❌ 误解 1：9router 有自己的 API Key

**错误认知**：9router 使用 `sk_9router` 作为 API Key

**正确理解**：9router 是代理工具，需要真实的后端 API Key

### ❌ 误解 2：9router 可以免费使用 AI 服务

**错误认知**：通过 9router 可以不用 API Key

**正确理解**：9router 只是转发请求，仍需要有效的 API Key

### ❌ 误解 3：`sk_9router` 是特殊的认证方式

**错误认知**：`sk_9router` 是 9router 的特殊认证

**正确理解**：这只是一个占位符，实际需要真实的 API Key

---

## 9router 的配置

### 9router 自身的配置

9router 本身可能有配置文件（如 `config.yaml`），其中需要配置：

```yaml
providers:
  - name: moonshot
    type: openai
    base_url: https://api.moonshot.cn/v1
    api_key: ${MOONSHOT_API_KEY}  # 从环境变量读取
```

### goclaw 的配置

goclaw 配置中的 API Key 会被传递给 9router：

```json
{
  "providers": {
    "9router": {
      "api_key": "sk-your-real-key",  // 这个 key 会被传递给 9router
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

---

## 验证配置

### 1. 测试 9router 连接

```bash
curl http://localhost:20128/v1/models \
  -H "Authorization: Bearer sk-your-real-moonshot-key"
```

应该返回可用的模型列表。

### 2. 测试 goclaw

```bash
./goclaw gateway run
```

查看日志，应该看到：
```
INFO  LLM provider resolved  provider=9router model=kimi-k2.5
INFO  Detected 9router proxy, enabling compatibility mode
```

### 3. 发送测试请求

```bash
curl -X POST http://localhost:28789/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "你好"}'
```

应该返回正常响应，不再出现 "No credentials" 错误。

---

## 故障排除

### 问题 1：仍然报 "No credentials" 错误

**检查**：
1. 确认 API Key 是否正确
2. 确认 API Key 是否有效（未过期）
3. 确认 9router 配置是否正确

**解决**：
```bash
# 直接测试 Moonshot API
curl https://api.moonshot.cn/v1/models \
  -H "Authorization: Bearer sk-your-key"

# 如果成功，说明 key 有效
# 如果失败，需要更换 key
```

### 问题 2：9router 无法连接到后端

**检查**：
1. 9router 是否正在运行
2. 9router 配置是否正确
3. 网络连接是否正常

**解决**：
```bash
# 查看 9router 日志
tail -f /path/to/9router/logs/app.log

# 重启 9router
systemctl restart 9router
```

### 问题 3：API Key 配额不足

**错误信息**：
```
"insufficient_quota" or "rate_limit_exceeded"
```

**解决**：
- 检查 API Key 的配额
- 升级 API 套餐
- 使用故障转移配置多个 key

---

## 最佳实践

### 1. 使用环境变量

不要在配置文件中硬编码 API Key：

```bash
# 设置环境变量
export MOONSHOT_API_KEY="sk-your-real-key"
```

```json
{
  "providers": {
    "9router": {
      "api_key": "${MOONSHOT_API_KEY}",
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

### 2. 使用故障转移

配置多个 API Key 以提高可用性：

```json
{
  "providers": {
    "failover": {
      "enabled": true
    },
    "profiles": [
      {
        "name": "key1",
        "provider": "9router",
        "api_key": "sk-key-1",
        "priority": 1
      },
      {
        "name": "key2",
        "provider": "9router",
        "api_key": "sk-key-2",
        "priority": 2
      }
    ]
  }
}
```

### 3. 监控 API 使用

定期检查 API 使用情况：
- 请求次数
- Token 消耗
- 错误率
- 配额剩余

---

## 总结

### 关键点

1. ✅ 9router 是**代理工具**，不是 API 提供商
2. ✅ 需要使用**真实的 API Key**（Moonshot/OpenAI）
3. ✅ `sk_9router` 只是**占位符**，不是有效的 key
4. ✅ API Key 会被传递给后端服务进行验证

### 正确配置

```json
{
  "providers": {
    "9router": {
      "api_key": "sk-your-real-moonshot-key",  // 真实的 key
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

---

**更新日期**: 2026-02-14
**问题**: "No credentials for provider: openai"
**解决方案**: 使用真实的 API Key 而不是 `sk_9router`
