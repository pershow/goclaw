# GoClaw Control UI - 快速开始

## 🎉 前端已成功移植！

GoClaw 现在拥有了一个基于 Web 的控制界面，类似于 OpenClaw 的 Control UI。

## 📸 当前功能

✅ **已实现**
- WebSocket 实时通信
- 聊天界面（消息发送/接收）
- 多视图导航（Chat、Channels、Sessions、Config）
- 连接状态指示
- 响应式布局
- 暗色/亮色主题支持

⏳ **待完善**
- Channel 配置界面（Discord、Telegram、Slack 等）
- Session 历史记录
- Agent 管理
- Tool 执行结果展示
- Markdown 渲染

## 🚀 快速启动

### 方式 1: 使用构建脚本（推荐）

**Windows:**
```bash
build-ui.bat
goclaw.exe gateway run --port 28789
```

**Linux/Mac:**
```bash
chmod +x build-ui.sh
./build-ui.sh
./goclaw gateway run --port 28789
```

然后打开浏览器访问: **http://localhost:28789/**

### 方式 2: 手动构建

```bash
# 1. 构建前端
cd ui
npm install
npm run build
cd ..

# 2. 复制到 gateway
mkdir -p gateway/dist
cp -r dist/control-ui gateway/dist/

# 3. 构建 Go 程序
go build -o goclaw.exe .

# 4. 运行
./goclaw.exe gateway run --port 28789
```

### 方式 3: 开发模式（前端热重载）

```bash
# 终端 1: 启动 Gateway
go run . gateway run --port 28789

# 终端 2: 启动前端开发服务器
cd ui
npm run dev
```

开发模式下访问: **http://localhost:5173/**

## 🌐 访问地址

- **Control UI**: http://localhost:28789/
- **WebSocket**: ws://localhost:28789/ws
- **Health Check**: http://localhost:28789/health
- **Channels API**: http://localhost:28789/api/channels

## 📁 项目结构

```
goclaw/
├── ui/                      # 前端源码
│   ├── src/
│   │   ├── main.ts         # 入口文件
│   │   ├── styles/         # 样式
│   │   └── ui/
│   │       ├── app.ts      # 主应用组件
│   │       ├── gateway.ts  # WebSocket 客户端
│   │       └── types.ts    # 类型定义
│   ├── package.json
│   └── vite.config.ts
├── gateway/
│   ├── control_ui.go       # UI 服务器（新增）
│   ├── server.go           # Gateway 服务器（已修改）
│   └── dist/
│       └── control-ui/     # 构建产物（嵌入到二进制）
├── build-ui.sh             # 构建脚本
└── build-ui.bat            # Windows 构建脚本
```

## 🛠️ 技术栈

- **前端**: Lit 3.3.2 (Web Components)
- **构建**: Vite 7.3.1
- **语言**: TypeScript
- **通信**: WebSocket (JSON-RPC 2.0)
- **后端**: Go (embed.FS)

## 📝 使用说明

### 发送消息
1. 在聊天界面输入框中输入消息
2. 按 Enter 或点击 Send 按钮发送
3. 消息会通过 WebSocket 发送到 Gateway

### 切换视图
点击顶部导航栏的按钮切换不同视图：
- **Chat**: 聊天界面
- **Channels**: 通道管理（待完善）
- **Sessions**: 会话管理（待完善）
- **Config**: 配置管理（待完善）

### 查看连接状态
顶部标题旁的圆点指示器：
- 🟢 绿色: 已连接
- 🔴 红色: 未连接

## 🔧 开发指南

### 添加新视图
1. 在 `ui/src/ui/views/` 创建新组件
2. 在 `app.ts` 中注册路由
3. 添加导航按钮

### 修改样式
编辑 `ui/src/styles/base.css`，使用 CSS 变量：
```css
--color-primary: #0066cc;
--color-bg: #ffffff;
--spacing-md: 16px;
```

### 调用 Gateway API
```typescript
// 在组件中
await this.gateway.call("method.name", { param: "value" });
```

## 🐛 故障排除

### UI 无法访问
- 检查 Gateway 是否正常启动
- 确认端口 28789 未被占用
- 查看控制台日志

### WebSocket 连接失败
- 检查防火墙设置
- 确认 WebSocket 路径正确 (`/ws`)
- 查看浏览器控制台错误

### 构建失败
- 确保 Node.js 版本 >= 18
- 删除 `node_modules` 重新安装
- 检查 Go 版本 >= 1.21

## 📚 更多信息

- 详细移植文档: `FRONTEND_MIGRATION.md`
- UI 开发文档: `ui/README.md`
- OpenClaw 原项目: https://github.com/openclaw/openclaw

## 🎯 下一步

建议按以下顺序完善功能：
1. Channel 配置界面（最常用）
2. Session 历史记录
3. Tool 执行结果展示
4. Markdown 渲染
5. Agent 管理

---

**享受使用 GoClaw Control UI！** 🚀
