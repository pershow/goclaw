# 快速测试指南

## 修复内容

### 1. 9router 406 错误修复
- **问题**: `extra_body.reasoning` 参数导致 406 错误
- **解决**: 9router 兼容模式下完全不发送额外参数
- **文件**: `providers/openai.go` (line 111, 433)

### 2. 子代理架构实现
- **功能**: 主代理调度，子代理异步执行任务
- **文件**: `agent/manager.go`, `agent/orchestrator.go`

---

## 测试步骤

### 1. 启动服务

```bash
./goclaw gateway run
```

**预期日志**:
```
INFO  LLM provider resolved  provider=9router model=if/kimi-k2.5
INFO  Detected 9router proxy, enabling compatibility mode
```

### 2. 发送测试消息

通过 WebSocket 或 HTTP 发送消息，应该看到：

```
DEBUG 9router compatibility mode: disabled reasoning_content and extra_body
```

### 3. 验证无 406 错误

之前的错误：
```
406 Not Acceptable {"message":"[iflow/kimi-k2.5] [406]: Unknown error"}
```

现在应该正常返回响应。

---

## 配置检查

### config.json 关键配置

```json
{
  "agents": {
    "defaults": {
      "model": "if/kimi-k2.5"  // ✅ 包含前缀
    }
  },
  "providers": {
    "9router": {
      "api_key": "sk_9router",  // ✅ 9router 令牌
      "base_url": "http://localhost:20128/v1",
      "streaming": true,
      "extra_body": {
        "reasoning": {"enabled": false}  // 兼容模式下会被忽略
      }
    }
  }
}
```

---

## 故障排查

### 如果仍然出现 406 错误

1. **检查编译**:
   ```bash
   go build -o goclaw.exe .
   ```

2. **检查 9router 是否运行**:
   ```bash
   curl http://localhost:20128/v1/models -H "Authorization: Bearer sk_9router"
   ```

3. **检查日志**:
   - 应该看到 "Detected 9router proxy, enabling compatibility mode"
   - 应该看到 "9router compatibility mode: disabled reasoning_content and extra_body"

### 如果出现 "No credentials" 错误

- 检查模型名称是否包含前缀: `if/kimi-k2.5`
- 检查 API key 是否为: `sk_9router`

---

## 可用模型

9router 支持的模型（需要包含前缀）:

- `if/kimi-k2.5` ⭐ (当前使用)
- `if/deepseek-r1`
- `if/deepseek-v3.2-chat`
- `ag/claude-opus-4-6-thinking`
- `gh/gpt-5.1`
- `kr/claude-sonnet-4.5`

---

**更新时间**: 2026-02-14
