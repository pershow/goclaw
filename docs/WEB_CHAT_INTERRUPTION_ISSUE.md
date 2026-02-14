# Web 界面对话中断问题分析

## 问题描述
用户反馈：在 Web 界面聊天时，交给 AI 一个任务后回复一两次就没反应了。

## 问题分析

### 日志分析
从日志中发现关键信息：

```
2026-02-14T21:33:47.762 === Calling LLM === {"messages_count": 50, ...}
2026-02-14T21:33:58.092 === LLM Response Received === {"content_length": 0, "tool_calls_count": 0}
2026-02-14T21:33:58.092 === Orchestrator Run End === {"final_messages_count": 50}
```

**关键发现**：
1. 消息数量达到 50 条
2. LLM 返回了空响应（content_length: 0, tool_calls_count: 0）
3. Orchestrator 结束运行

### 根本原因

#### 1. 迭代次数限制
- 配置中 `max_iterations: 15`
- 每次工具调用算一次迭代
- 达到限制后，如果 LLM 返回空响应，对话就会结束

#### 2. LLM 返回空响应
可能的原因：
- **上下文过长**：消息数量达到 50 条，可能接近上下文窗口限制
- **模型认为任务完成**：Kimi 模型可能认为没有更多内容需要输出
- **Token 限制**：达到了某个内部限制

#### 3. Orchestrator 停止逻辑
```go
// agent/orchestrator.go:200-211
// Agent would stop here. Check for follow-up messages.
if len(followUpMessages) > 0 {
    pendingMessages = append(pendingMessages, followUpMessages...)
    continue
}
// No more messages, exit
break
```

当 LLM 返回空内容且没有工具调用时，如果没有 follow-up 消息，orchestrator 会退出循环。

---

## 解决方案

### 1. 增加 max_iterations（已实施）

**修改前**：
```json
{
  "agents": {
    "defaults": {
      "max_iterations": 15
    }
  }
}
```

**修改后**：
```json
{
  "agents": {
    "defaults": {
      "max_iterations": 30
    }
  }
}
```

**效果**：允许更多的工具调用迭代，减少因达到限制而中断的情况。

### 2. 其他可能的优化

#### A. 增加 context_tokens
```json
{
  "agents": {
    "defaults": {
      "context_tokens": 200000  // Kimi k2.5 支持 200k 上下文
    }
  }
}
```

#### B. 启用历史消息限制
```json
{
  "agents": {
    "defaults": {
      "limit_history_turns": 20  // 限制保留的历史轮次
    }
  }
}
```

#### C. 调整 max_tokens
```json
{
  "agents": {
    "defaults": {
      "max_tokens": 4096  // 减少单次响应的 token 数
    }
  }
}
```

---

## 监控建议

### 1. 添加日志
在 orchestrator 中添加更详细的日志：
- 当前迭代次数
- 剩余迭代次数
- 上下文 token 使用情况

### 2. 错误提示
当达到 max_iterations 时，向用户显示友好的提示：
```
"任务执行已达到最大迭代次数限制。您可以继续对话或重新开始。"
```

### 3. 空响应处理
当 LLM 返回空响应时，可以：
- 记录警告日志
- 向用户显示提示
- 自动重试一次

---

## 对比 OpenClaw

OpenClaw 的配置：
```json
{
  "agents": {
    "defaults": {
      "maxConcurrent": 4,
      "subagents": {
        "maxConcurrent": 8
      }
    }
  }
}
```

OpenClaw 可能有不同的迭代限制策略或更好的空响应处理。

---

## 测试建议

### 1. 长对话测试
- 发送需要多次工具调用的复杂任务
- 观察是否能完成整个任务
- 检查日志中的迭代次数

### 2. 边界测试
- 测试接近 30 次迭代的任务
- 观察达到限制时的行为
- 验证错误提示是否友好

### 3. 上下文测试
- 测试长上下文对话
- 观察消息压缩和历史限制的效果
- 验证性能和响应质量

---

## 当前状态

✅ **已修改**：
- `max_iterations`: 15 → 30

✅ **服务状态**：
- 已重启（PID: 4000）
- 监听端口：28789
- 配置已生效

⏳ **待验证**：
- 用户测试长对话是否正常
- 观察是否还会出现中断

---

## 建议的后续改进

### 1. 代码层面
- 添加空响应重试机制
- 改进迭代限制的错误提示
- 添加上下文使用情况的监控

### 2. 配置层面
- 根据实际使用情况调整 max_iterations
- 考虑启用 limit_history_turns
- 优化 context_tokens 配置

### 3. 用户体验
- 显示当前任务进度
- 提供"继续"按钮
- 显示剩余迭代次数

---

**更新时间**: 2026-02-14 21:40
**问题状态**: 已缓解（增加了 max_iterations）
**需要验证**: 用户实际使用反馈
