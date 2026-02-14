package types

import (
	"context"
	"testing"
	"time"
)

func TestRetryStrategy_ShouldRetry(t *testing.T) {
	classifier := NewSimpleErrorClassifier()
	strategy := NewDefaultRetryStrategy(classifier)

	tests := []struct {
		name     string
		err      error
		attempt  int
		expected bool
	}{
		{
			name:     "nil error should not retry",
			err:      nil,
			attempt:  0,
			expected: false,
		},
		{
			name:     "timeout error should retry",
			err:      &mockError{msg: "request timeout"},
			attempt:  0,
			expected: true,
		},
		{
			name:     "rate limit error should retry",
			err:      &mockError{msg: "rate limit exceeded"},
			attempt:  0,
			expected: true,
		},
		{
			name:     "auth error should not retry",
			err:      &mockError{msg: "invalid api key"},
			attempt:  0,
			expected: false,
		},
		{
			name:     "max retries exceeded",
			err:      &mockError{msg: "timeout"},
			attempt:  3,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldRetry(tt.err, tt.attempt)
			if result != tt.expected {
				t.Errorf("ShouldRetry() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRetryStrategy_GetDelay(t *testing.T) {
	classifier := NewSimpleErrorClassifier()
	strategy := NewRetryStrategy(3, 1*time.Second, 10*time.Second, 2.0, classifier)

	tests := []struct {
		name     string
		attempt  int
		expected time.Duration
	}{
		{
			name:     "first retry",
			attempt:  0,
			expected: 1 * time.Second,
		},
		{
			name:     "second retry",
			attempt:  1,
			expected: 2 * time.Second,
		},
		{
			name:     "third retry",
			attempt:  2,
			expected: 4 * time.Second,
		},
		{
			name:     "max delay reached",
			attempt:  4,
			expected: 10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.GetDelay(tt.attempt)
			if result != tt.expected {
				t.Errorf("GetDelay(%d) = %v, want %v", tt.attempt, result, tt.expected)
			}
		})
	}
}

func TestRetryStrategy_Retry(t *testing.T) {
	classifier := NewSimpleErrorClassifier()
	strategy := NewRetryStrategy(2, 10*time.Millisecond, 100*time.Millisecond, 2.0, classifier)

	t.Run("success on first attempt", func(t *testing.T) {
		attempts := 0
		err := strategy.Retry(context.Background(), func() error {
			attempts++
			return nil
		})

		if err != nil {
			t.Errorf("Retry() error = %v, want nil", err)
		}
		if attempts != 1 {
			t.Errorf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("success after retry", func(t *testing.T) {
		attempts := 0
		err := strategy.Retry(context.Background(), func() error {
			attempts++
			if attempts < 2 {
				return &mockError{msg: "timeout"}
			}
			return nil
		})

		if err != nil {
			t.Errorf("Retry() error = %v, want nil", err)
		}
		if attempts != 2 {
			t.Errorf("attempts = %d, want 2", attempts)
		}
	})

	t.Run("non-retryable error", func(t *testing.T) {
		attempts := 0
		err := strategy.Retry(context.Background(), func() error {
			attempts++
			return &mockError{msg: "invalid api key"}
		})

		if err == nil {
			t.Error("Retry() error = nil, want error")
		}
		if attempts != 1 {
			t.Errorf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		attempts := 0
		err := strategy.Retry(context.Background(), func() error {
			attempts++
			return &mockError{msg: "timeout"}
		})

		if err == nil {
			t.Error("Retry() error = nil, want error")
		}
		if attempts != 3 { // 1 initial + 2 retries
			t.Errorf("attempts = %d, want 3", attempts)
		}
	})
}

func TestRetryWithResult(t *testing.T) {
	classifier := NewSimpleErrorClassifier()
	strategy := NewRetryStrategy(2, 10*time.Millisecond, 100*time.Millisecond, 2.0, classifier)

	t.Run("success with result", func(t *testing.T) {
		attempts := 0
		result, err := RetryWithResult(context.Background(), strategy, func() (string, error) {
			attempts++
			if attempts < 2 {
				return "", &mockError{msg: "timeout"}
			}
			return "success", nil
		})

		if err != nil {
			t.Errorf("RetryWithResult() error = %v, want nil", err)
		}
		if result != "success" {
			t.Errorf("result = %s, want success", result)
		}
		if attempts != 2 {
			t.Errorf("attempts = %d, want 2", attempts)
		}
	})
}

// mockError 模拟错误
type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}
