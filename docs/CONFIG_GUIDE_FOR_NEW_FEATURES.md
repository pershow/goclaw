# 新功能配置指南

## 配置文件修改说明

本指南说明如何配置 goclaw 以使用新实现的功能：
1. 子 Agent 异步执行
2. 9router 代理兼容

---

## 1. 使用 9router 代理

### 方案 A：直接修改 base_url（推荐）

如果你使用 9router 作为本地代理，只需修改 `config.json` 中的 `base_url`：

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-your-api-key-here",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "max_retries": 3,
      "extra_body": {
        "reasoning": {
          "enabled": false
        }
      }
    }
  }
}
```

**说明**：
- 将 `base_url` 从 `https://api.moonshot.cn/v1` 改为 `http://localhost:20128/v1`
- 系统会自动检测 `:20128` 端口并启用 9router 兼容模式
- 无需其他配置，完全自动

### 方案 B：使用其他端口的 9router

如果你的 9router 运行在其他端口（如 8080）：

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-your-api-key-here",
      "base_url": "http://localhost:8080/v1",
      "timeout": 600,
      "max_retries": 3
    }
  }
}
```

**注意**：目前自动检测仅支持 `:20128` 端口。如果使用其他端口，需要手动修改代码中的检测逻辑。

### 验证 9router 是否生效

启动 goclaw 后，查看日志：

```
INFO  Detected 9router proxy, enabling compatibility mode  base_url=http://localhost:20128/v1
```

如果看到这条日志，说明 9router 兼容模式已启用。

---

## 2. 配置子 Agent

子 Agent 的配置在 `agents.defaults.subagents` 部分：

```json
{
  "agents": {
    "defaults": {
      "model": "kimi-k2.5",
      "max_iterations": 15,
      "temperature": 1,
      "max_tokens": 8192,
      "subagents": {
        "max_concurrent": 8,
        "archive_after_minutes": 60,
        "model": "kimi-k2.5",
        "timeout_seconds": 300
      }
    }
  }
}
```

### 配置项说明

| 配置项 | 说明 | 默认值 | 推荐值 |
|--------|------|--------|--------|
| `max_concurrent` | 最大并发子 agent 数量 | 8 | 4-16 |
| `archive_after_minutes` | 子 agent 结果归档时间（分钟） | 60 | 30-120 |
| `model` | 子 agent 使用的模型 | 继承主 agent | 可用更便宜的模型 |
| `timeout_seconds` | 子 agent 超时时间（秒） | 300 | 60-600 |

### 调整建议

**场景 1：高并发任务**
```json
{
  "subagents": {
    "max_concurrent": 16,
    "archive_after_minutes": 30,
    "model": "kimi-k2.5",
    "timeout_seconds": 180
  }
}
```

**场景 2：长时间任务**
```json
{
  "subagents": {
    "max_concurrent": 4,
    "archive_after_minutes": 120,
    "model": "kimi-k2.5",
    "timeout_seconds": 600
  }
}
```

**场景 3：成本优化**
```json
{
  "subagents": {
    "max_concurrent": 8,
    "archive_after_minutes": 60,
    "model": "moonshot-v1-8k",
    "timeout_seconds": 300
  }
}
```

---

## 3. 完整配置示例

### 示例 1：使用 9router + 子 Agent

```json
{
  "workspace": {
    "path": "C:\\Users\\Administrator\\.goclaw\\workspace"
  },
  "agents": {
    "defaults": {
      "model": "kimi-k2.5",
      "max_iterations": 15,
      "temperature": 1,
      "max_tokens": 8192,
      "subagents": {
        "max_concurrent": 8,
        "archive_after_minutes": 60,
        "model": "kimi-k2.5",
        "timeout_seconds": 300
      }
    }
  },
  "providers": {
    "openai": {
      "api_key": "sk-your-api-key-here",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "max_retries": 3,
      "extra_body": {
        "reasoning": {
          "enabled": false
        }
      }
    }
  },
  "gateway": {
    "enabled": true,
    "host": "0.0.0.0",
    "port": 28789
  }
}
```

### 示例 2：直连 Moonshot API + 子 Agent

```json
{
  "workspace": {
    "path": "C:\\Users\\Administrator\\.goclaw\\workspace"
  },
  "agents": {
    "defaults": {
      "model": "kimi-k2.5",
      "max_iterations": 15,
      "temperature": 1,
      "max_tokens": 8192,
      "subagents": {
        "max_concurrent": 8,
        "archive_after_minutes": 60,
        "model": "kimi-k2.5",
        "timeout_seconds": 300
      }
    }
  },
  "providers": {
    "openai": {
      "api_key": "sk-your-moonshot-api-key",
      "base_url": "https://api.moonshot.cn/v1",
      "timeout": 600,
      "max_retries": 3,
      "extra_body": {
        "reasoning": {
          "enabled": false
        }
      }
    }
  },
  "gateway": {
    "enabled": true,
    "host": "0.0.0.0",
    "port": 28789
  }
}
```

---

## 4. 使用子 Agent 功能

### 通过 API 调用

```bash
curl -X POST http://localhost:28789/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "请使用子 agent 分析这个文件",
    "tool_calls": [{
      "name": "sessions_spawn",
      "params": {
        "task": "分析 main.go 的代码质量",
        "label": "code-review",
        "cleanup": "keep"
      }
    }]
  }'
```

### 通过 WebSocket

```javascript
const ws = new WebSocket('ws://localhost:28789/ws');

ws.send(JSON.stringify({
  type: 'tool_call',
  tool: 'sessions_spawn',
  params: {
    task: '分析这个项目的架构',
    label: 'architecture-review',
    cleanup: 'delete'
  }
}));
```

### 通过 CLI

```bash
./goclaw chat "请创建一个子 agent 来分析代码"
```

---

## 5. 环境变量配置（可选）

如果不想修改配置文件，可以使用环境变量：

### Windows

```cmd
set GOCLAW_OPENAI_BASE_URL=http://localhost:20128/v1
set GOCLAW_OPENAI_API_KEY=sk-your-api-key
./goclaw gateway run
```

### Linux/Mac

```bash
export GOCLAW_OPENAI_BASE_URL=http://localhost:20128/v1
export GOCLAW_OPENAI_API_KEY=sk-your-api-key
./goclaw gateway run
```

---

## 6. 配置验证

### 检查配置是否正确

```bash
./goclaw config validate
```

### 查看当前配置

```bash
./goclaw config show
```

### 测试 9router 连接

```bash
curl http://localhost:20128/v1/models
```

应该返回可用的模型列表。

---

## 7. 常见问题

### Q1: 9router 兼容模式没有启用？

**检查**：
1. 确认 `base_url` 包含 `:20128`
2. 查看启动日志是否有 "Detected 9router proxy" 消息
3. 确认 9router 服务正在运行

**解决**：
```bash
# 检查 9router 是否运行
curl http://localhost:20128/v1/models

# 查看 goclaw 日志
./goclaw gateway run --log-level debug
```

### Q2: 子 Agent 创建失败？

**检查**：
1. 确认 `subagents` 配置存在
2. 检查 `max_concurrent` 是否达到上限
3. 查看日志中的错误信息

**解决**：
```json
{
  "subagents": {
    "max_concurrent": 16,  // 增加并发数
    "timeout_seconds": 600  // 增加超时时间
  }
}
```

### Q3: 仍然出现 406 错误？

**可能原因**：
1. 9router 版本不兼容
2. 使用了非标准端口
3. 代理配置错误

**解决**：
1. 更新 9router 到最新版本
2. 检查 9router 配置
3. 尝试直连 API（临时禁用 9router）

---

## 8. 性能调优

### 高性能配置

```json
{
  "agents": {
    "defaults": {
      "max_iterations": 20,
      "subagents": {
        "max_concurrent": 16,
        "archive_after_minutes": 30,
        "timeout_seconds": 180
      }
    }
  },
  "providers": {
    "openai": {
      "timeout": 300,
      "max_retries": 5
    }
  }
}
```

### 低延迟配置

```json
{
  "agents": {
    "defaults": {
      "max_iterations": 10,
      "subagents": {
        "max_concurrent": 4,
        "archive_after_minutes": 15,
        "timeout_seconds": 60
      }
    }
  },
  "providers": {
    "openai": {
      "timeout": 120,
      "max_retries": 2
    }
  }
}
```

---

## 9. 监控和日志

### 启用详细日志

```bash
./goclaw gateway run --log-level debug
```

### 查看子 Agent 状态

```bash
# 查看运行中的子 agent
./goclaw agent list

# 查看子 agent 历史
./goclaw agent history
```

### 日志位置

- Windows: `C:\Users\Administrator\.goclaw\logs\`
- Linux/Mac: `~/.goclaw/logs/`

---

## 10. 迁移指南

### 从旧版本升级

如果你从旧版本升级，需要：

1. **备份配置文件**
```bash
cp config.json config.json.backup
```

2. **添加 subagents 配置**
```json
{
  "agents": {
    "defaults": {
      "subagents": {
        "max_concurrent": 8,
        "archive_after_minutes": 60,
        "model": "kimi-k2.5",
        "timeout_seconds": 300
      }
    }
  }
}
```

3. **如果使用 9router，修改 base_url**
```json
{
  "providers": {
    "openai": {
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

4. **重启服务**
```bash
./goclaw gateway restart
```

---

## 总结

### 最小配置（仅使用子 Agent）

```json
{
  "agents": {
    "defaults": {
      "subagents": {
        "max_concurrent": 8,
        "archive_after_minutes": 60
      }
    }
  }
}
```

### 完整配置（9router + 子 Agent）

```json
{
  "agents": {
    "defaults": {
      "model": "kimi-k2.5",
      "subagents": {
        "max_concurrent": 8,
        "archive_after_minutes": 60,
        "model": "kimi-k2.5",
        "timeout_seconds": 300
      }
    }
  },
  "providers": {
    "openai": {
      "api_key": "sk-xxx",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "max_retries": 3
    }
  }
}
```

### 关键点

1. ✅ 9router 自动检测，只需修改 `base_url`
2. ✅ 子 Agent 配置在 `agents.defaults.subagents`
3. ✅ 无需额外配置，开箱即用
4. ✅ 查看日志确认功能是否启用

---

**更新日期**: 2026-02-14
**适用版本**: goclaw v1.0.0+
