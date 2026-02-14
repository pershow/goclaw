# 9router 406 错误最终解决方案

## 完成时间
2026-02-14 21:35

---

## 问题根源

### 错误现象
```
{"error":{"message":"[iflow/kimi-k2.5] [406]: Unknown error"}}
```

### 真正原因
**API Key 不正确**

- ❌ 错误配置: `"api_key": "sk-ea61013aa9db2a61-njakaf-0b10f6c0"`
- ✅ 正确配置: `"api_key": "sk_9router"`

---

## 问题排查过程

### 1. 初步怀疑：参数兼容性
- 怀疑 `extra_body.reasoning` 参数导致 406
- 修改代码禁用所有额外参数
- **结果**：问题依然存在 ❌

### 2. 怀疑：模型名称
- 测试 `if/kimi-k2` vs `if/kimi-k2.5`
- `if/kimi-k2` 使用真实密钥可以工作 ✅
- `if/kimi-k2.5` 使用真实密钥返回 406 ❌
- **结果**：部分正确，但不是根本原因

### 3. 怀疑：Temperature 参数
- 添加 Kimi 模型的 temperature=0.6 限制
- **结果**：问题依然存在 ❌

### 4. 对比 OpenClaw 配置
- 发现 OpenClaw 使用 `"apiKey": "sk_9router"`
- goclaw 使用 `"api_key": "sk-ea61013aa9db2a61-njakaf-0b10f6c0"`
- **关键发现**：API Key 不同！

### 5. 验证测试
```bash
# 使用真实密钥 - 失败
curl -H "Authorization: Bearer sk-ea61013aa9db2a61-njakaf-0b10f6c0" \
  -d '{"model":"if/kimi-k2.5","messages":[{"role":"user","content":"hi"}]}'
# 结果: 406 错误

# 使用 sk_9router - 成功
curl -H "Authorization: Bearer sk_9router" \
  -d '{"model":"if/kimi-k2.5","messages":[{"role":"user","content":"hi"}]}'
# 结果: 正常返回 ✅
```

---

## 解决方案

### 修改配置文件

**config.json**:
```json
{
  "providers": {
    "9router": {
      "api_key": "sk_9router",  // ✅ 使用默认密钥
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "streaming": true,
      "extra_body": {
        "reasoning": {
          "enabled": false
        }
      }
    }
  },
  "profiles": [
    {
      "name": "9router-primary",
      "provider": "9router",
      "api_key": "sk_9router",  // ✅ 使用默认密钥
      "base_url": "http://localhost:20128/v1",
      "priority": 1
    }
  ]
}
```

---

## 技术细节

### 9router 认证机制

9router 使用**统一认证令牌**：
- 客户端使用 `sk_9router` 作为认证令牌
- 9router 后端管理真实的 API 密钥
- 不同的密钥可能有不同的模型访问权限

### 为什么真实密钥不工作？

可能的原因：
1. 该密钥没有 `if/kimi-k2.5` 的访问权限
2. 该密钥是特定用户/组织的密钥，有访问限制
3. 9router 配置中该密钥没有绑定 Infoflow 后端

### 为什么 if/kimi-k2 可以工作？

- `if/kimi-k2` 可能是公开模型或基础模型
- `if/kimi-k2.5` 可能需要特定权限或配置

---

## 验证步骤

### 1. 启动服务
```bash
./goclaw gateway run
```

### 2. 预期日志
```
INFO  config loaded  model=if/kimi-k2.5
INFO  Detected 9router proxy, enabling compatibility mode
INFO  Gateway listening on 0.0.0.0:28789
```

### 3. 测试请求
发送消息到 WebSocket，应该正常返回响应，不再出现 406 错误。

---

## 总结

### 问题本质
- ❌ 不是代码问题
- ❌ 不是参数兼容性问题
- ❌ 不是模型名称问题
- ✅ **是 API Key 配置问题**

### 关键教训
1. 对比参考实现（OpenClaw）的配置非常重要
2. API Key 可能有不同的访问权限
3. 9router 使用统一认证令牌 `sk_9router`

### 最终配置
- **API Key**: `sk_9router`
- **模型**: `if/kimi-k2.5`
- **Base URL**: `http://localhost:20128/v1`
- **Streaming**: `true`

---

## 子代理架构状态

✅ **已完整实现**：
1. `handleSubagentSpawn()` - 处理子代理生成
2. `getOrCreateSubagent()` - 获取或创建子代理
3. `runSubagent()` - 异步执行任务
4. `sendToSession()` - 消息注入
5. Context 传递 session_key

---

**更新日期**: 2026-02-14 21:35
**状态**: ✅ 问题已解决
**服务状态**: ✅ 正常运行
