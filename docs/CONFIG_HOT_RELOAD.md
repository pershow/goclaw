# 配置热重载功能

## 概述

goclaw 现在支持配置热重载功能，允许在不重启服务的情况下动态更新配置。当配置文件发生变化时，系统会自动检测并重新加载配置，应用到运行中的组件。

## 功能特性

- **自动检测**: 监听配置文件变化，自动触发重载
- **防抖机制**: 500ms 防抖延迟，避免频繁重载
- **配置验证**: 重载前验证新配置的有效性
- **组件更新**: 自动更新 Gateway、Channel Manager、Session Manager 等组件
- **客户端通知**: 向所有连接的 WebSocket 客户端广播配置重载事件
- **错误处理**: 如果新配置无效，保持使用旧配置

## 使用方法

### 1. 启动 Gateway 时自动启用

当使用配置文件启动 Gateway 时，热重载功能会自动启用：

```bash
# 使用默认配置文件 (~/.goclaw/config.json)
goclaw gateway run

# 使用指定配置文件
goclaw gateway run --config /path/to/config.json
```

启动日志会显示：
```
Config hot reload enabled, watching: /home/user/.goclaw/config.json
```

### 2. 修改配置文件

直接编辑配置文件，保存后系统会自动检测并重载：

```bash
# 编辑配置文件
vim ~/.goclaw/config.json

# 或使用任何文本编辑器
code ~/.goclaw/config.json
```

### 3. 手动触发重载

也可以使用 CLI 命令手动触发配置重载：

```bash
goclaw gateway reload
```

## 支持的配置项

以下配置项支持热重载：

### Gateway 配置
- WebSocket 端口和地址
- 认证设置
- 超时配置

### Channel 配置
- 启用/禁用通道
- 通道认证信息
- Webhook 配置

### Session 配置
- 会话作用域
- 重置策略
- 存储路径

### Agent 配置
- 默认模型
- 最大迭代次数
- 温度和 token 限制

## 配置变更流程

1. **文件监听**: fsnotify 监听配置文件所在目录
2. **防抖处理**: 500ms 内的多次变更合并为一次重载
3. **加载配置**: 读取并解析新配置文件
4. **验证配置**: 验证新配置的有效性
5. **更新组件**: 依次更新各个组件的配置
6. **广播通知**: 向所有 WebSocket 客户端发送 `config_reloaded` 事件
7. **记录日志**: 记录重载成功或失败信息

## 日志示例

### 成功重载
```
INFO  Config file changed  event=WRITE file=/home/user/.goclaw/config.json
INFO  Reloading configuration  path=/home/user/.goclaw/config.json
INFO  Configuration changed, reloading components...
INFO  Gateway handling config reload
INFO  WebSocket config changed, updating...
INFO  WebSocket config updated
INFO  Config reload notification broadcasted  connections=2
INFO  Configuration reloaded successfully
```

### 配置验证失败
```
ERROR Failed to reload config  error="gateway port must be between 1 and 65535"
```

## WebSocket 事件

当配置重载成功时，所有连接的 WebSocket 客户端会收到以下事件：

```json
{
  "type": "event",
  "event": "config_reloaded",
  "payload": {
    "timestamp": 1707901878486
  }
}
```

前端可以监听此事件并做出相应处理（如刷新配置、显示通知等）。

## 注意事项

1. **配置验证**: 如果新配置无效，系统会保持使用旧配置，不会中断服务
2. **部分重启**: 某些配置项（如端口变更）可能需要重启服务才能完全生效
3. **文件权限**: 确保配置文件有正确的读取权限
4. **编辑器兼容**: 支持大多数编辑器（包括 vim、vscode、nano 等）
5. **服务模式**: 如果 Gateway 作为系统服务运行，热重载同样有效

## 最佳实践

1. **备份配置**: 修改前备份当前配置文件
2. **验证语法**: 使用 JSON 验证工具检查语法
3. **逐步修改**: 一次修改少量配置项，便于排查问题
4. **查看日志**: 修改后查看日志确认重载成功
5. **测试验证**: 重载后测试相关功能是否正常

## 故障排查

### 配置未重载

1. 检查日志中是否有 "Config hot reload enabled" 消息
2. 确认配置文件路径正确
3. 检查文件权限
4. 查看是否有错误日志

### 配置重载失败

1. 检查 JSON 语法是否正确
2. 验证配置值是否在有效范围内
3. 查看详细错误日志
4. 恢复备份配置

### 部分配置未生效

某些配置项可能需要重启服务：

```bash
# 重启 Gateway 服务
goclaw gateway restart

# 或手动停止后启动
goclaw gateway stop
goclaw gateway start
```

## 开发者信息

### 相关文件

- `config/watcher.go`: 配置监听器实现
- `config/loader.go`: 配置加载和热重载接口
- `gateway/reload.go`: Gateway 配置重载处理
- `cli/commands/gateway.go`: CLI 命令集成

### 扩展热重载

要为新组件添加热重载支持：

```go
// 注册配置变更处理函数
config.OnConfigChange(func(oldCfg, newCfg *config.Config) error {
    // 检查配置是否变化
    if oldCfg.YourComponent != newCfg.YourComponent {
        // 更新组件配置
        yourComponent.UpdateConfig(newCfg.YourComponent)
    }
    return nil
})
```

## 性能影响

- **CPU**: 文件监听占用极少 CPU 资源
- **内存**: 增加约 1-2MB 内存占用
- **延迟**: 配置变更后 500ms 内完成重载
- **并发**: 不影响正在处理的请求

## 与 OpenClaw 对齐

此功能与 OpenClaw 的配置热重载功能保持一致：

- ✅ 自动文件监听
- ✅ 防抖机制
- ✅ 配置验证
- ✅ 组件更新
- ✅ 客户端通知

## 未来改进

- [ ] 支持配置文件的部分重载（只重载变更的部分）
- [ ] 添加配置变更历史记录
- [ ] 支持配置回滚功能
- [ ] 提供配置变更的 Web UI
- [ ] 支持远程配置推送
