package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWatcher(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Write initial config (with valid provider and tools)
	initialConfig := `{
  "agents": {
    "defaults": {
      "model": "openrouter:anthropic/claude-opus-4-5",
      "max_iterations": 15,
      "temperature": 1.0,
      "max_tokens": 8192
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "sk-test-key-1234567890"
    }
  },
  "tools": {
    "shell": {
      "enabled": true,
      "denied_cmds": ["rm -rf", "dd", "mkfs"]
    }
  },
  "gateway": {
    "host": "localhost",
    "port": 8080,
    "websocket": {
      "host": "0.0.0.0",
      "port": 28789,
      "path": "/ws"
    }
  }
}`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	// Load initial config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Gateway.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Gateway.Port)
	}

	// Create watcher
	watcher, err := NewWatcher(configPath)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Track config changes
	changeDetected := make(chan bool, 1)
	watcher.OnChange(func(oldCfg, newCfg *Config) error {
		if oldCfg.Gateway.Port != newCfg.Gateway.Port {
			changeDetected <- true
		}
		return nil
	})

	// Start watching
	watcher.Start()

	// Wait a bit for watcher to initialize
	time.Sleep(100 * time.Millisecond)

	// Modify config
	updatedConfig := `{
  "agents": {
    "defaults": {
      "model": "openrouter:anthropic/claude-opus-4-5",
      "max_iterations": 15,
      "temperature": 1.0,
      "max_tokens": 8192
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "sk-test-key-1234567890"
    }
  },
  "tools": {
    "shell": {
      "enabled": true,
      "denied_cmds": ["rm -rf", "dd", "mkfs"]
    }
  },
  "gateway": {
    "host": "localhost",
    "port": 9090,
    "websocket": {
      "host": "0.0.0.0",
      "port": 28789,
      "path": "/ws"
    }
  }
}`

	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to write updated config: %v", err)
	}

	// Wait for change detection (with timeout)
	select {
	case <-changeDetected:
		t.Log("Config change detected successfully")
	case <-time.After(2 * time.Second):
		t.Error("Config change not detected within timeout")
	}

	// Verify global config was updated
	currentCfg := Get()
	if currentCfg.Gateway.Port != 9090 {
		t.Errorf("Expected port 9090 after reload, got %d", currentCfg.Gateway.Port)
	}
}

func TestWatcherDebounce(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Write initial config (with valid provider and tools)
	initialConfig := `{
  "agents": {
    "defaults": {
      "model": "openrouter:anthropic/claude-opus-4-5",
      "max_iterations": 15,
      "temperature": 1.0,
      "max_tokens": 8192
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "sk-test-key-1234567890"
    }
  },
  "tools": {
    "shell": {
      "enabled": true,
      "denied_cmds": ["rm -rf", "dd", "mkfs"]
    }
  },
  "gateway": {
    "host": "localhost",
    "port": 8080,
    "websocket": {
      "host": "0.0.0.0",
      "port": 28789,
      "path": "/ws"
    }
  }
}`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	// Load initial config
	if _, err := Load(configPath); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create watcher
	watcher, err := NewWatcher(configPath)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Track number of change events
	changeCount := 0
	var mu sync.Mutex
	watcher.OnChange(func(oldCfg, newCfg *Config) error {
		mu.Lock()
		changeCount++
		mu.Unlock()
		return nil
	})

	// Start watching
	watcher.Start()

	// Wait for watcher to initialize
	time.Sleep(100 * time.Millisecond)

	// Write multiple times rapidly (should be debounced)
	for i := 0; i < 5; i++ {
		port := 9000 + i
		updatedConfig := fmt.Sprintf(`{
  "agents": {
    "defaults": {
      "model": "openrouter:anthropic/claude-opus-4-5",
      "max_iterations": 15,
      "temperature": 1.0,
      "max_tokens": 8192
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "sk-test-key-1234567890"
    }
  },
  "tools": {
    "shell": {
      "enabled": true,
      "denied_cmds": ["rm -rf", "dd", "mkfs"]
    }
  },
  "gateway": {
    "host": "localhost",
    "port": %d,
    "websocket": {
      "host": "0.0.0.0",
      "port": 28789,
      "path": "/ws"
    }
  }
}`, port)
		if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for debounce to settle
	time.Sleep(1 * time.Second)

	// Should have triggered only once or twice due to debouncing
	mu.Lock()
	count := changeCount
	mu.Unlock()

	if count > 3 {
		t.Errorf("Expected at most 3 change events due to debouncing, got %d", count)
	}

	t.Logf("Change events triggered: %d (debouncing working)", count)
}
