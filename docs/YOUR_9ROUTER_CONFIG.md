# 你的配置文件修改方案（使用独立 9router 配置）

## 当前配置

你当前的 `config.json` 使用：
- Provider: OpenAI (实际指向 Moonshot)
- API Key: `sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV`
- Base URL: `https://api.moonshot.cn/v1`

根据你遇到的 406 错误和使用 9router 的需求，现在可以使用独立的 9router 配置。

---

## 推荐方案：使用独立的 9router 配置

### 修改步骤

1. 打开 `config.json`
2. 在 `providers` 部分添加 `9router` 配置
3. 保持其他配置不变（作为备用）

### 完整配置文件

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
  "channels": {
    "telegram": {
      "enabled": false,
      "token": "",
      "allowed_ids": []
    },
    "whatsapp": {
      "enabled": false,
      "bridge_url": "",
      "allowed_ids": []
    },
    "feishu": {
      "enabled": true,
      "app_id": "cli_a90615c106e1dbb5",
      "app_secret": "jb9R7Qnplmrap8dEgg1vzeWMJ7VAwlwq",
      "encrypt_key": "",
      "verification_token": "pershow",
      "event_mode": "long_connection",
      "webhook_port": 8765,
      "allowed_ids": []
    },
    "qq": {
      "enabled": false,
      "app_id": "",
      "app_secret": "",
      "allowed_ids": []
    },
    "wework": {
      "enabled": false,
      "corp_id": "",
      "agent_id": "",
      "secret": "",
      "token": "",
      "encoding_aes_key": ""
    },
    "dingtalk": {
      "enabled": false,
      "client_id": "",
      "secret": "",
      "allowed_ids": []
    },
    "infoflow": {
      "enabled": false,
      "webhook_url": "",
      "token": "",
      "aes_key": "",
      "webhook_port": 18766,
      "allowed_ids": [],
      "accounts": {
        "bot1": {
          "enabled": false,
          "name": "Infoflow Bot 1",
          "webhook_url": "",
          "token": "",
          "aes_key": "",
          "webhook_port": 18766,
          "allowed_ids": []
        }
      }
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "",
      "base_url": "",
      "timeout": 600,
      "max_retries": 3,
      "extra_body": {
        "reasoning": {
          "enabled": false
        }
      }
    },
    "openai": {
      "api_key": "",
      "base_url": "",
      "timeout": 600,
      "extra_body": {
        "reasoning": {
          "enabled": false
        }
      }
    },
    "anthropic": {
      "api_key": "",
      "base_url": "",
      "timeout": 600
    },
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
    }
  },
  "gateway": {
    "enabled": true,
    "host": "0.0.0.0",
    "port": 28789,
    "read_timeout": 30,
    "write_timeout": 30,
    "websocket": {
      "host": "0.0.0.0",
      "port": 28789,
      "path": "/ws",
      "enable_auth": false,
      "auth_token": "",
      "ping_interval": "30s",
      "pong_timeout": "60s",
      "read_timeout": "60s",
      "write_timeout": "10s"
    }
  },
  "tools": {
    "filesystem": {
      "allowed_paths": [
        "C:\\Users\\Administrator\\.goclaw\\workspace",
        "C:\\Users\\Administrator\\.goclaw\\skills",
        "C:\\Users\\Administrator\\.goclaw\\memory"
      ],
      "denied_paths": [
        "C:\\Windows",
        "C:\\Program Files"
      ]
    },
    "shell": {
      "enabled": true,
      "allowed_cmds": [],
      "denied_cmds": ["rm -rf", "dd", "mkfs", "format"],
      "timeout": 30,
      "working_dir": "",
      "sandbox": {
        "enabled": false
      }
    },
    "web": {
      "search_api_key": "",
      "search_engine": "travily",
      "timeout": 10
    },
    "browser": {
      "enabled": true,
      "headless": true,
      "timeout": 30
    }
  },
  "memory": {
    "enabled": true,
    "backend": "builtin",
    "builtin": {
      "enabled": true,
      "database_path": "",
      "auto_index": true
    },
    "qmd": {
      "command": "qmd",
      "enabled": false,
      "include_default": true,
      "paths": [
        {
          "name": "notes",
          "path": "~/notes",
          "pattern": "**/*.md"
        },
        {
          "name": "docs",
          "path": "~/Documents",
          "pattern": "**/*.md"
        }
      ],
      "sessions": {
        "enabled": false,
        "export_dir": "~/.goclaw/sessions/export",
        "retention_days": 30
      },
      "update": {
        "interval": "5m",
        "on_boot": true,
        "embed_interval": "60m",
        "command_timeout": "30s",
        "update_timeout": "120s"
      },
      "limits": {
        "max_results": 6,
        "max_snippet_chars": 700,
        "timeout_ms": 4000
      }
    }
  },
  "skills": {
    "enabled": true
  },
  "approvals": {
    "enabled": false
  }
}
```

---

## 关键修改点

### 1. 添加 9router 配置

在 `providers` 部分添加：

```json
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
}
```

### 2. 清空其他 provider 配置（可选）

将 `openai` 的配置清空，避免冲突：

```json
"openai": {
  "api_key": "",
  "base_url": "",
  "timeout": 600,
  "extra_body": {
    "reasoning": {
      "enabled": false
    }
  }
}
```

---

## 快速修改命令

### 方式 1：手动编辑

1. 打开 `config.json`
2. 找到 `"providers"` 部分
3. 在 `"anthropic"` 后面添加逗号
4. 添加以下内容：

```json
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
}
```

5. 将 `openai` 的 `api_key` 和 `base_url` 清空
6. 保存文件

### 方式 2：使用 PowerShell

```powershell
# 备份配置文件
Copy-Item config.json config.json.backup

# 使用文本编辑器打开
notepad config.json
```

然后手动添加 9router 配置。

---

## 验证配置

### 1. 检查 JSON 格式

```bash
# 使用 Python 验证 JSON 格式
python -m json.tool config.json
```

或者使用在线工具：https://jsonlint.com/

### 2. 启动 goclaw

```bash
./goclaw gateway run
```

### 3. 查看日志

应该看到：

```
INFO  LLM provider resolved  provider=9router model=kimi-k2.5
INFO  Detected 9router proxy, enabling compatibility mode  base_url=http://localhost:20128/v1
```

### 4. 测试请求

```bash
curl -X POST http://localhost:28789/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "你好"}'
```

---

## 配置优先级

系统会按以下顺序检测 provider：

1. 模型前缀（如 `9router:kimi-k2.5`）
2. OpenRouter（如果有 API Key）
3. Anthropic（如果有 API Key）
4. **9router（如果有 API Key 或 Base URL）** ← 你的配置
5. OpenAI（如果有 API Key）
6. Moonshot（如果有 API Key）

由于你清空了其他 provider 的 API Key，系统会自动使用 9router。

---

## 如果需要切换回 Moonshot

只需修改 `openai` 配置：

```json
"openai": {
  "api_key": "sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV",
  "base_url": "https://api.moonshot.cn/v1",
  "timeout": 600
}
```

并清空 9router 配置：

```json
"9router": {
  "api_key": "",
  "base_url": "",
  "timeout": 600
}
```

---

## 使用故障转移（高级）

如果你想同时使用 9router 和 Moonshot，并在 9router 失败时自动切换：

```json
{
  "providers": {
    "failover": {
      "enabled": true,
      "strategy": "round_robin",
      "default_cooldown": "5m",
      "circuit_breaker": {
        "failure_threshold": 3,
        "timeout": "30s"
      }
    },
    "profiles": [
      {
        "name": "9router-primary",
        "provider": "9router",
        "api_key": "sk_9router",
        "base_url": "http://localhost:20128/v1",
        "priority": 1
      },
      {
        "name": "moonshot-backup",
        "provider": "openai",
        "api_key": "sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV",
        "base_url": "https://api.moonshot.cn/v1",
        "priority": 2
      }
    ]
  }
}
```

---

## 故障排除

### 问题 1：找不到 9router 配置

**错误信息**：
```
ERROR: no LLM provider API key configured
```

**解决**：
- 确认 `9router` 配置已添加
- 确认 JSON 格式正确（逗号、括号）
- 重启 goclaw 服务

### 问题 2：仍然使用 openai

**原因**：
- `openai` 的 `api_key` 没有清空
- 系统优先使用了 openai

**解决**：
```json
"openai": {
  "api_key": "",  // 清空
  "base_url": ""  // 清空
}
```

### 问题 3：连接 9router 失败

**检查**：
```bash
# 测试 9router 连接
curl http://localhost:20128/v1/models \
  -H "Authorization: Bearer sk_9router"
```

**解决**：
- 启动 9router 服务
- 检查端口是否正确
- 检查防火墙设置

---

## 总结

### 核心修改

只需在 `providers` 中添加：

```json
"9router": {
  "api_key": "sk_9router",
  "base_url": "http://localhost:20128/v1",
  "timeout": 600,
  "streaming": true
}
```

### 优势

1. ✅ 配置清晰，不与其他 provider 混淆
2. ✅ 自动使用 `sk_9router` 作为 API Key
3. ✅ 自动启用 9router 兼容模式
4. ✅ 支持故障转移和备用 provider
5. ✅ 易于切换和维护

---

**修改日期**: 2026-02-14
**适用场景**: 使用 9router 代理访问 Moonshot/Kimi API
**配置类型**: 独立 9router 配置（推荐）
