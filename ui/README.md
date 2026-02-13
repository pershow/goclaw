# GoClaw Control UI

## 已完成的功能

### 基础设施 ✅
- ✅ Vite + TypeScript + Lit 构建配置
- ✅ 基础样式系统（CSS 变量、主题支持）
- ✅ Go embed 静态文件服务
- ✅ Gateway HTTP/WebSocket 服务器集成

### 核心组件 ✅
- ✅ WebSocket Gateway 客户端（自动重连、消息队列）
- ✅ 主应用组件（GoClawApp）
- ✅ 基础类型定义
- ✅ 聊天界面（消息显示、输入框）
- ✅ 导航系统（Chat、Channels、Sessions、Config）

### 服务端集成 ✅
- ✅ Control UI 静态文件服务
- ✅ HTTP 和 WebSocket 端口复用
- ✅ 健康检查端点
- ✅ Channels API 端点

## 待完成的功能

### 高优先级
- ⏳ 完整的 Channel 管理界面（Discord、Telegram、Slack、WhatsApp 等）
- ⏳ Session 管理和历史记录
- ⏳ Agent 配置界面
- ⏳ 实时消息流处理
- ⏳ Tool 执行卡片显示

### 中优先级
- ⏳ 配置表单动态生成
- ⏳ Markdown 渲染
- ⏳ 代码高亮
- ⏳ 日志查看器
- ⏳ 使用统计图表

### 低优先级
- ⏳ Canvas/A2UI 支持
- ⏳ 暗色主题完善
- ⏳ 移动端响应式优化
- ⏳ 国际化支持

## 如何使用

### 构建 UI
```bash
cd ui
npm install
npm run build
```

### 复制到 Gateway
```bash
cp -r dist/control-ui gateway/dist/
```

### 运行 Gateway
```bash
go build -o goclaw.exe .
./goclaw.exe gateway run --port 28789
```

### 访问 UI
打开浏览器访问: http://localhost:28789/

## 技术栈

- **前端框架**: Lit (Web Components)
- **构建工具**: Vite 7.3.1
- **语言**: TypeScript
- **样式**: CSS (CSS 变量)
- **通信**: WebSocket (JSON-RPC 2.0)
- **后端**: Go (embed.FS)

## 目录结构

```
ui/
├── src/
│   ├── main.ts              # 入口文件
│   ├── styles/
│   │   └── base.css         # 基础样式
│   └── ui/
│       ├── app.ts           # 主应用组件
│       ├── gateway.ts       # WebSocket 客户端
│       └── types.ts         # 类型定义
├── index.html               # HTML 模板
├── package.json             # 依赖配置
├── tsconfig.json            # TypeScript 配置
└── vite.config.ts           # Vite 配置

gateway/
├── control_ui.go            # UI 服务器
├── server.go                # Gateway 服务器
└── dist/
    └── control-ui/          # 构建产物（嵌入）
```

## 下一步

1. 实现完整的 Channel 配置界面
2. 添加 Session 历史记录功能
3. 实现 Tool 执行结果展示
4. 添加 Markdown 和代码高亮
5. 完善错误处理和加载状态
