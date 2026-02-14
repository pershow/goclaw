# 9router 配置指南

## 配置方式

现在 goclaw 支持独立的 9router 配置，类似 moonshot 的配置方式。

---

## 方式 1：使用独立的 9router 配置（推荐）

### 配置文件

在 `config.json` 中添加 `9router` 配置：

```json
{
  "agents": {
    "defaults": {
      "model": "kimi-k2.5",
      "max_iterations": 15,
      "temperature": 1,
      "max_tokens": 8192
    }
  },
  "providers": {
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
  }
}
```

### 配置说明

| 字段 | 说明 | 默认值 | 必填 |
|------|------|--------|------|
| `api_key` | 9router API Key | `sk_9router` | 否 |
| `base_url` | 9router 服务地址 | `http://localhost:20128/v1` | 否 |
| `timeout` | 请求超时时间（秒） | 600 | 否 |
| `streaming` | 是否启用流式输出 | `true` | 否 |
| `extra_body` | 额外的请求参数 | `{}` | 否 |

### 特点

- ✅ 自动使用 `sk_9router` 作为 API Key
- ✅ 自动使用 `http://localhost:20128/v1` 作为默认地址
- ✅ 自动启用 9router 兼容模式（禁用 reasoning_content）
- ✅ 配置简洁，开箱即用

---

## 方式 2：使用模型前缀

### 配置文件

```json
{
  "agents": {
    "defaults": {
      "model": "9router:kimi-k2.5"
    }
  },
  "providers": {
    "9router": {
      "api_key": "sk_9router",
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

使用 `9router:` 前缀明确指定使用 9router 提供商。

---

## 方式 3：最小配置

如果你只配置了 9router，可以省略其他 provider 配置：

```json
{
  "agents": {
    "defaults": {
      "model": "kimi-k2.5"
    }
  },
  "providers": {
    "9router": {
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

系统会自动：
- 使用 `sk_9router` 作为 API Key
- 检测到只有 9router 配置，自动使用它

---

## 完整配置示例

### 示例 1：仅使用 9router

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
    "9router": {
      "api_key": "sk_9router",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "streaming": true
    }
  },
  "gateway": {
    "enabled": true,
    "host": "0.0.0.0",
    "port": 28789
  }
}
```

### 示例 2：9router + Moonshot 备用

```json
{
  "agents": {
    "defaults": {
      "model": "kimi-k2.5"
    }
  },
  "providers": {
    "9router": {
      "api_key": "sk_9router",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600
    },
    "moonshot": {
      "api_key": "sk-your-moonshot-key",
      "base_url": "https://api.moonshot.cn/v1",
      "timeout": 600
    }
  }
}
```

系统会优先使用 9router（因为在检测顺序中更靠前）。

### 示例 3：使用故障转移

```json
{
  "agents": {
    "defaults": {
      "model": "kimi-k2.5"
    }
  },
  "providers": {
    "failover": {
      "enabled": true,
      "strategy": "round_robin",
      "default_cooldown": "5m"
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
        "provider": "moonshot",
        "api_key": "sk-your-moonshot-key",
        "base_url": "https://api.moonshot.cn/v1",
        "priority": 2
      }
    ]
  }
}
```

---

## 从旧配置迁移

### 旧配置（修改 openai 的 base_url）

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-your-key",
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

### 新配置（使用独立的 9router）

```json
{
  "providers": {
    "9router": {
      "api_key": "sk_9router",
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

### 迁移步骤

1. **备份配置**
   ```bash
   cp config.json config.json.backup
   ```

2. **删除或注释 openai 配置**
   ```json
   {
     "providers": {
       "openai": {
         "api_key": "",
         "base_url": ""
       }
     }
   }
   ```

3. **添加 9router 配置**
   ```json
   {
     "providers": {
       "9router": {
         "api_key": "sk_9router",
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

## 验证配置

### 1. 检查配置文件

```bash
grep -A 5 "9router" config.json
```

应该显示：
```json
"9router": {
  "api_key": "sk_9router",
  "base_url": "http://localhost:20128/v1",
  "timeout": 600
}
```

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

如果返回正常响应，说明配置成功！

---

## 自定义端口

如果你的 9router 运行在其他端口：

```json
{
  "providers": {
    "9router": {
      "api_key": "sk_9router",
      "base_url": "http://localhost:8080/v1"
    }
  }
}
```

系统会自动检测端口并启用兼容模式。

---

## 自定义 API Key

如果你的 9router 使用自定义 API Key：

```json
{
  "providers": {
    "9router": {
      "api_key": "your-custom-key",
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

---

## 故障排除

### 问题 1：连接被拒绝

**检查**：
```bash
# 确认 9router 正在运行
curl http://localhost:20128/v1/models \
  -H "Authorization: Bearer sk_9router"
```

**解决**：
- 启动 9router 服务
- 检查端口是否正确
- 检查防火墙设置

### 问题 2：认证失败

**检查**：
- 确认 API Key 是否正确（默认为 `sk_9router`）
- 查看 9router 日志

**解决**：
```json
{
  "providers": {
    "9router": {
      "api_key": "sk_9router"  // 确保与 9router 配置一致
    }
  }
}
```

### 问题 3：仍然出现 406 错误

**可能原因**：
- 9router 版本过旧
- 配置未生效

**解决**：
1. 更新 9router 到最新版本
2. 重启 goclaw 服务
3. 查看详细日志：
   ```bash
   ./goclaw gateway run --log-level debug
   ```

---

## 与其他 Provider 的对比

| Provider | API Key | Base URL | 兼容模式 |
|----------|---------|----------|----------|
| openai | 真实 OpenAI Key | `https://api.openai.com/v1` | 否 |
| moonshot | 真实 Moonshot Key | `https://api.moonshot.cn/v1` | 否 |
| 9router | `sk_9router` | `http://localhost:20128/v1` | 是 |

---

## 优势

### 使用独立 9router 配置的优势

1. ✅ **配置清晰**：不会与 openai 配置混淆
2. ✅ **自动兼容**：自动启用 9router 兼容模式
3. ✅ **默认值**：提供合理的默认值，配置更简单
4. ✅ **易于切换**：可以轻松在 9router 和其他 provider 之间切换
5. ✅ **故障转移**：支持与其他 provider 配合使用故障转移

---

## 总结

### 最简配置

```json
{
  "providers": {
    "9router": {
      "base_url": "http://localhost:20128/v1"
    }
  }
}
```

### 推荐配置

```json
{
  "providers": {
    "9router": {
      "api_key": "sk_9router",
      "base_url": "http://localhost:20128/v1",
      "timeout": 600,
      "streaming": true
    }
  }
}
```

### 关键点

1. ✅ 使用独立的 `9router` 配置项
2. ✅ API Key 默认为 `sk_9router`
3. ✅ Base URL 默认为 `http://localhost:20128/v1`
4. ✅ 自动启用兼容模式，无需额外配置
5. ✅ 支持所有标准 provider 功能（故障转移、流式输出等）

---

**更新日期**: 2026-02-14
**适用版本**: goclaw v1.0.0+
