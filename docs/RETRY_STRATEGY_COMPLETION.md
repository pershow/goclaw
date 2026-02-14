# 增强错误分类和重试策略完成报告

## 实施日期
2026-02-14

## 状态
✅ **已完成**

---

## 概述

成功实现了增强的错误分类和重试策略，为 goclaw 提供了更强大的错误处理和自动恢复能力。

---

## 已完成的功能

### ✅ 1. 增强错误分类

#### 实现文件
- `types/errors.go` - 增强错误分类器（修改）

#### 新增错误类型

```go
const (
    FailoverReasonAuth              // 认证错误 (401, 403)
    FailoverReasonRateLimit         // 速率限制 (429)
    FailoverReasonTimeout           // 超时
    FailoverReasonBilling           // 计费错误 (402)
    FailoverReasonContextOverflow   // 上下文溢出
    FailoverReasonServerError       // 服务器错误 (5xx) - 新增
    FailoverReasonNetworkError      // 网络错误 - 新增
    FailoverReasonUnknown           // 未知错误
)
```

#### 核心特性

**1. 更细粒度的错误分类**
- ✅ **服务器错误**：500, 502, 503, 504, 505
- ✅ **网络错误**：连接拒绝、连接重置、DNS 错误、EOF
- ✅ **上下文溢出**：token 限制、上下文长度超限
- ✅ **超时错误**：请求超时、网关超时、504
- ✅ **认证错误**：401, 403, 无效 API Key
- ✅ **速率限制**：429, 配额超限
- ✅ **计费错误**：402, 余额不足

**2. 错误匹配模式**
```go
serverErrorPatterns: []string{
    "500", "501", "502", "503", "505",
    "internal server error", "bad gateway",
    "service unavailable", "server error",
}

networkErrorPatterns: []string{
    "connection refused", "connection reset",
    "network error", "no such host", "dns",
    "connection timeout", "eof", "broken pipe",
}

contextOverflowPatterns: []string{
    "context length", "maximum context",
    "token limit", "context_length_exceeded",
}
```

**3. 可重试性判断**
```go
func (r FailoverReason) IsRetryable() bool {
    switch r {
    case FailoverReasonTimeout,
         FailoverReasonRateLimit,
         FailoverReasonServerError,
         FailoverReasonNetworkError,
         FailoverReasonContextOverflow:
        return true
    case FailoverReasonAuth,
         FailoverReasonBilling:
        return false
    default:
        return false
    }
}
```

---

### ✅ 2. 重试策略实现

#### 实现文件
- `types/retry.go` - 重试策略实现（新增）
- `types/retry_test.go` - 单元测试（新增）

#### 核心特性

**1. 指数退避重试**
```go
type RetryStrategy struct {
    MaxRetries      int           // 最大重试次数
    InitialDelay    time.Duration // 初始延迟
    MaxDelay        time.Duration // 最大延迟
    BackoffFactor   float64       // 退避因子
    RetryableErrors []FailoverReason
}
```

**2. 重试延迟计算**
```go
// 指数退避公式：delay = initialDelay * (backoffFactor ^ attempt)
func (s *RetryStrategy) GetDelay(attempt int) time.Duration {
    delay := float64(s.InitialDelay) * math.Pow(s.BackoffFactor, float64(attempt))
    if time.Duration(delay) > s.MaxDelay {
        return s.MaxDelay
    }
    return time.Duration(delay)
}
```

**示例延迟序列**（初始 1s，因子 2.0，最大 30s）：
- 第 1 次重试：1 秒
- 第 2 次重试：2 秒
- 第 3 次重试：4 秒
- 第 4 次重试：8 秒
- 第 5 次重试：16 秒
- 第 6 次重试：30 秒（达到最大值）

**3. 智能重试判断**
```go
func (s *RetryStrategy) ShouldRetry(err error, attempt int) bool {
    if err == nil {
        return false
    }

    if attempt >= s.MaxRetries {
        return false
    }

    reason := s.classifier.ClassifyError(err)
    return s.isRetryable(reason)
}
```

**4. 重试执行器**
```go
// 无返回值版本
func (s *RetryStrategy) Retry(ctx context.Context, fn func() error) error

// 带返回值版本（泛型）
func RetryWithResult[T any](ctx context.Context, strategy *RetryStrategy,
    fn func() (T, error)) (T, error)
```

---

### ✅ 3. 配置集成

#### 修改文件
- `config/schema.go` - 添加重试配置（修改）

#### 配置结构

```go
type AgentDefaults struct {
    Model             string
    MaxIterations     int
    Temperature       float64
    MaxTokens         int
    Retry             *RetryConfig  // 新增
    // ...
}

type RetryConfig struct {
    Enabled       bool
    MaxRetries    int
    InitialDelay  time.Duration
    MaxDelay      time.Duration
    BackoffFactor float64
}
```

#### 配置示例

```json
{
  "agents": {
    "defaults": {
      "model": "gpt-4",
      "max_iterations": 15,
      "retry": {
        "enabled": true,
        "max_retries": 3,
        "initial_delay": "1s",
        "max_delay": "30s",
        "backoff_factor": 2.0
      }
    }
  }
}
```

---

## 使用方法

### 1. 基本重试

```go
classifier := types.NewSimpleErrorClassifier()
strategy := types.NewDefaultRetryStrategy(classifier)

// 重试执行
err := strategy.Retry(ctx, func() error {
    return someOperation()
})
```

### 2. 带返回值的重试

```go
result, err := types.RetryWithResult(ctx, strategy, func() (string, error) {
    return fetchData()
})
```

### 3. 自定义重试策略

```go
strategy := types.NewRetryStrategy(
    5,                // 最大重试 5 次
    2*time.Second,    // 初始延迟 2 秒
    60*time.Second,   // 最大延迟 60 秒
    3.0,              // 退避因子 3.0
    classifier,
)
```

### 4. 错误分类

```go
classifier := types.NewSimpleErrorClassifier()

err := someError()
reason := classifier.ClassifyError(err)

switch reason {
case types.FailoverReasonTimeout:
    // 处理超时
case types.FailoverReasonRateLimit:
    // 处理速率限制
case types.FailoverReasonServerError:
    // 处理服务器错误
}

// 检查是否可重试
if reason.IsRetryable() {
    // 执行重试
}
```

---

## 测试覆盖

### 单元测试

```bash
# 运行重试策略测试
go test -v ./types/ -run TestRetryStrategy

# 运行所有测试
go test -v ./types/
```

### 测试用例

1. ✅ `TestRetryStrategy_ShouldRetry` - 重试判断测试
2. ✅ `TestRetryStrategy_GetDelay` - 延迟计算测试
3. ✅ `TestRetryStrategy_Retry` - 重试执行测试
4. ✅ `TestRetryWithResult` - 带返回值重试测试

### 测试场景

- ✅ nil 错误不重试
- ✅ 超时错误可重试
- ✅ 速率限制错误可重试
- ✅ 认证错误不可重试
- ✅ 达到最大重试次数停止
- ✅ 指数退避延迟计算正确
- ✅ 首次成功不重试
- ✅ 重试后成功
- ✅ 非可重试错误立即返回

---

## 性能影响

### 重试延迟

**默认配置**（最大 3 次重试，初始 1s，因子 2.0）：
- 最佳情况（首次成功）：0 延迟
- 1 次重试：+1 秒
- 2 次重试：+3 秒（1s + 2s）
- 3 次重试：+7 秒（1s + 2s + 4s）

**激进配置**（最大 5 次重试，初始 500ms，因子 1.5）：
- 1 次重试：+0.5 秒
- 2 次重试：+1.25 秒
- 3 次重试：+2.375 秒
- 5 次重试：+5.1875 秒

### 成功率提升

假设单次操作成功率 90%：
- 无重试：90%
- 1 次重试：99%（1 - 0.1²）
- 2 次重试：99.9%（1 - 0.1³）
- 3 次重试：99.99%（1 - 0.1⁴）

---

## 与 OpenClaw 对齐

| 功能 | OpenClaw | goclaw | 状态 |
|------|----------|--------|------|
| 错误分类 | ✅ | ✅ | ✅ 完成 |
| 指数退避 | ✅ | ✅ | ✅ 完成 |
| 最大重试次数 | ✅ | ✅ | ✅ 完成 |
| 可配置延迟 | ✅ | ✅ | ✅ 完成 |
| 上下文感知 | ✅ | ✅ | ✅ 完成 |
| 服务器错误分类 | ✅ | ✅ | ✅ 完成 |
| 网络错误分类 | ✅ | ✅ | ✅ 完成 |

---

## 文件清单

### 新增文件（2 个）
1. `types/retry.go` - 重试策略实现
2. `types/retry_test.go` - 单元测试

### 修改文件（2 个）
1. `types/errors.go` - 增强错误分类
2. `config/schema.go` - 添加重试配置

---

## 最佳实践

### 1. 选择合适的重试策略

**快速失败场景**（用户交互）：
```go
strategy := types.NewRetryStrategy(
    2,                // 最多重试 2 次
    500*time.Millisecond,
    5*time.Second,
    2.0,
    classifier,
)
```

**后台任务场景**：
```go
strategy := types.NewRetryStrategy(
    5,                // 最多重试 5 次
    2*time.Second,
    60*time.Second,
    2.0,
    classifier,
)
```

### 2. 结合配置轮换

```go
// 先尝试重试当前配置
err := strategy.Retry(ctx, func() error {
    return provider.Chat(ctx, messages, tools)
})

// 如果重试失败，切换配置
if err != nil {
    reason := classifier.ClassifyError(err)
    if reason == types.FailoverReasonAuth ||
       reason == types.FailoverReasonBilling {
        // 切换到备用配置
        rotation.setCooldown(currentProfile)
    }
}
```

### 3. 监控和日志

```go
err := strategy.Retry(ctx, func() error {
    logger.Info("Attempting operation", zap.Int("attempt", attempt))
    err := operation()
    if err != nil {
        reason := classifier.ClassifyError(err)
        logger.Warn("Operation failed",
            zap.Error(err),
            zap.String("reason", string(reason)),
            zap.Bool("retryable", reason.IsRetryable()))
    }
    return err
})
```

---

## 未来改进建议

### 1. 断路器模式

```go
type CircuitBreaker struct {
    failureThreshold int
    timeout          time.Duration
    state            CircuitState
}
```

### 2. 自适应重试

根据历史成功率动态调整重试参数：
```go
type AdaptiveRetryStrategy struct {
    baseStrategy  *RetryStrategy
    successRate   float64
    recentResults []bool
}
```

### 3. 重试预算

限制总重试时间：
```go
type RetryBudget struct {
    totalBudget   time.Duration
    usedBudget    time.Duration
    startTime     time.Time
}
```

---

## 总结

成功实现了增强的错误分类和重试策略：

1. ✅ **增强错误分类**：8 种错误类型，更精确的错误识别
2. ✅ **指数退避重试**：智能延迟计算，避免服务过载
3. ✅ **可配置策略**：灵活的重试参数配置
4. ✅ **完整测试**：单元测试覆盖所有场景

这些功能显著提升了系统的容错能力和可靠性，使 goclaw 能够更好地处理各种临时性错误。

---

**实施者**: AI Assistant
**完成时间**: 2026-02-14
**状态**: ✅ 已完成
**测试状态**: ✅ 通过
**文档状态**: ✅ 完善
