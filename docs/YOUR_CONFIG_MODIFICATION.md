# 你的配置文件修改方案

## 当前配置分析

你当前的 `config.json` 使用的是：
- Provider: Moonshot API (kimi-k2.5)
- Base URL: `https://api.moonshot.cn/v1`
- API Key: `sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV`

根据你遇到的 406 错误，你应该是在使用 9router 代理。

---

## 方案 1：使用 9router 代理（推荐）

### 修改步骤

1. 打开 `config.json`
2. 找到 `providers.openai` 部分
3. 将 `base_url` 修改为 9router 地址

### 修改前

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV",
      "base_url": "https://api.moonshot.cn/v1",
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

### 修改后

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV",
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

**关键变化**：
- `base_url` 从 `https://api.moonshot.cn/v1` 改为 `http://localhost:20128/v1`
- 系统会自动检测并启用 9router 兼容模式
- 不会再出现 406 错误

---

## 方案 2：直连 Moonshot API（不使用 9router）

如果你不想使用 9router，保持当前配置不变即可：

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV",
      "base_url": "https://api.moonshot.cn/v1",
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

但需要确保：
1. 关闭 9router 服务
2. 或者不设置 HTTP_PROXY 环境变量

---

## 完整配置文件示例

### 使用 9router + 子 Agent

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
      "api_key": "sk-JuzFPoZyeONIj7R4iks5SlfwduDhoLeAYuwxDvlKbiLrhDxV",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "max_retries": 3,
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

## 快速修改命令

### Windows PowerShell

```powershell
# 备份配置文件
Copy-Item config.json config.json.backup

# 使用 PowerShell 修改配置
$config = Get-Content config.json | ConvertFrom-Json
$config.providers.openai.base_url = "http://localhost:20128/v1"
$config | ConvertTo-Json -Depth 10 | Set-Content config.json
```

### Linux/Mac

```bash
# 备份配置文件
cp config.json config.json.backup

# 使用 sed 修改配置
sed -i 's|https://api.moonshot.cn/v1|http://localhost:20128/v1|g' config.json
```

### 手动修改

1. 打开 `config.json`
2. 找到第 94 行：`"base_url": "https://api.moonshot.cn/v1",`
3. 改为：`"base_url": "http://localhost:20128/v1",`
4. 保存文件

---

## 验证修改

### 1. 检查配置文件

```bash
# 查看 base_url 是否修改成功
grep "base_url" config.json
```

应该显示：
```
"base_url": "http://localhost:20128/v1",
```

### 2. 启动 goclaw

```bash
./goclaw gateway run
```

### 3. 查看日志

应该看到：
```
INFO  Detected 9router proxy, enabling compatibility mode  base_url=http://localhost:20128/v1
```

### 4. 测试请求

```bash
curl -X POST http://localhost:28789/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "你好"}'
```

如果不再出现 406 错误，说明配置成功！

---

## 故障排除

### 问题 1：仍然出现 406 错误

**检查**：
```bash
# 确认 9router 正在运行
curl http://localhost:20128/v1/models

# 查看 goclaw 日志
./goclaw gateway run --log-level debug
```

### 问题 2：连接被拒绝

**可能原因**：
- 9router 未启动
- 端口号错误

**解决**：
```bash
# 检查 9router 进程
ps aux | grep 9router

# 检查端口占用
netstat -ano | findstr 20128
```

### 问题 3：API Key 无效

**检查**：
- 确认 API Key 是否正确
- 确认 9router 配置中的 API Key

---

## 总结

### 最简单的修改方式

只需修改一行：

```json
"base_url": "http://localhost:20128/v1"
```

就可以：
- ✅ 解决 406 错误
- ✅ 启用 9router 兼容模式
- ✅ 正常使用所有功能

### 不需要修改的部分

- ❌ API Key 不需要改
- ❌ 其他配置不需要改
- ❌ 不需要添加额外配置

---

**修改日期**: 2026-02-14
**适用场景**: 使用 9router 代理访问 Moonshot API
