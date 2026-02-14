# 管理模式 Agent 测试指南

## 已实现的功能

### ✅ 系统提示已配置
主 Agent 现在被配置为**任务管理者**，会：
1. 分析任务复杂度
2. 简单任务直接处理
3. 复杂任务拆解并创建子 Agent
4. 监控子 Agent 进度
5. 汇总结果

---

## 测试场景

### 场景 1：简单任务（应该直接回答）

**测试输入**：
```
这个函数是做什么的？
```

**预期行为**：
- 主 Agent 直接分析并回答
- 不创建子 Agent
- 快速响应

---

### 场景 2：复杂任务（应该创建子 Agent）

**测试输入**：
```
请分析 providers/openai.go、providers/factory.go 和 agent/manager.go 这三个文件的代码质量。
```

**预期行为**：
1. 主 Agent 分析：这是复杂任务，需要分析 3 个文件
2. 主 Agent 回复：
   ```
   我已经分析了你的任务，这需要分析 3 个文件。我将创建 3 个子 Agent 并行处理：
   1. 子 Agent A：分析 providers/openai.go
   2. 子 Agent B：分析 providers/factory.go
   3. 子 Agent C：分析 agent/manager.go

   预计几分钟内完成，我会持续监控进度。
   ```
3. 使用 `sessions_spawn` 工具创建 3 个子 Agent
4. 等待子 Agent 完成
5. 汇总结果并回复

**验证方法**：
查看日志是否有 `sessions_spawn` 调用：
```bash
tail -f C:\Users\Administrator\.goclaw\logs\goclaw.log | grep "sessions_spawn"
```

---

### 场景 3：进度查询

**前提**：先执行场景 2，创建了子 Agent

**测试输入**：
```
之前的任务进行到哪儿了？
```

**预期行为**：
1. 主 Agent 使用 `sessions_list` 查看所有子 Agent
2. 主 Agent 使用 `session_status` 查询每个子 Agent 状态
3. 主 Agent 回复：
   ```
   当前进度：
   - ✅ 已完成：1 个任务
   - ⏳ 进行中：2 个任务
   - ⏸️ 等待中：0 个任务

   详细状态：
   - 子 Agent A (openai.go): 已完成 ✅
   - 子 Agent B (factory.go): 执行中 ⏳
   - 子 Agent C (manager.go): 执行中 ⏳
   ```

---

### 场景 4：并行任务

**测试输入**：
```
请同时执行以下任务：
1. 读取 config.json 并总结配置
2. 分析 agent/orchestrator.go 的主要功能
3. 检查 providers/ 目录下有哪些文件
```

**预期行为**：
1. 主 Agent 识别这是 3 个独立任务
2. 创建 3 个子 Agent 并行执行
3. 告知用户已创建子 Agent
4. 等待所有子 Agent 完成
5. 汇总 3 个任务的结果

---

### 场景 5：复杂多步骤任务

**测试输入**：
```
我需要重构整个 providers 目录：
1. 分析所有 provider 文件
2. 识别重复代码
3. 提取公共接口
4. 生成重构建议
```

**预期行为**：
1. 主 Agent 分析：这是复杂的多步骤任务
2. 拆解为多个子任务
3. 创建多个子 Agent 处理不同步骤
4. 按顺序或并行执行（取决于依赖关系）
5. 汇总所有结果

---

## 监控命令

### 实时查看日志
```bash
tail -f C:\Users\Administrator\.goclaw\logs\goclaw.log
```

### 查看子 Agent 相关日志
```bash
tail -f C:\Users\Administrator\.goclaw\logs\goclaw.log | grep -E "(sessions_spawn|Subagent|session_status|sessions_list)"
```

### 查看工具调用
```bash
tail -f C:\Users\Administrator\.goclaw\logs\goclaw.log | grep -E "(Execute Tool|tool_calls_count)"
```

---

## 预期日志输出

### 创建子 Agent 时
```
INFO  Execute Tool Calls Start  {"count": 1}
INFO  Tool registered  {"tool": "sessions_spawn"}
INFO  Subagent spawn handled  {"run_id": "...", "subagent_id": "...", "task": "..."}
INFO  Starting subagent execution  {"run_id": "...", "session_key": "...", "task": "..."}
```

### 查询进度时
```
INFO  Execute Tool Calls Start  {"count": 1}
INFO  Tool: sessions_list
INFO  Tool: session_status
```

---

## 故障排查

### 问题 1：主 Agent 不创建子 Agent

**可能原因**：
- 系统提示未生效
- LLM 认为任务不够复杂
- 工具调用失败

**解决方法**：
1. 检查配置是否正确加载：
   ```bash
   grep "system_prompt" C:\Users\Administrator\.goclaw\config.json
   ```
2. 明确要求使用子 Agent：
   ```
   请使用 sessions_spawn 工具创建子 Agent 来处理这个任务。
   ```

### 问题 2：子 Agent 创建失败

**检查日志**：
```bash
tail -100 C:\Users\Administrator\.goclaw\logs\goclaw.log | grep ERROR
```

**常见错误**：
- 工具未注册
- 参数错误
- Registry 未初始化

### 问题 3：无法查询进度

**可能原因**：
- `sessions_list` 或 `session_status` 工具未注册
- Session 管理器未初始化

**验证工具注册**：
```bash
grep "Tool registered" C:\Users\Administrator\.goclaw\logs\goclaw.log | grep session
```

---

## 成功标准

### ✅ 管理模式工作正常的标志

1. **任务分析**：主 Agent 会说"我已经分析了你的任务..."
2. **子 Agent 创建**：日志中有 "Subagent spawn handled"
3. **进度报告**：能够回答"任务进行到哪儿了"
4. **结果汇总**：最后给出整合后的结果
5. **并行执行**：多个子 Agent 同时运行

### ❌ 需要改进的标志

1. 主 Agent 自己执行所有任务（没有创建子 Agent）
2. 无法查询进度
3. 子 Agent 创建失败
4. 结果没有汇总

---

## 当前配置

- **服务状态**: ✅ 运行中（PID: 2039）
- **端口**: 28789
- **系统提示**: ✅ 已配置管理模式
- **工具**: ✅ sessions_spawn, sessions_list, session_status 已注册
- **max_iterations**: 30

---

## 下一步

1. **测试简单任务**：验证主 Agent 能直接处理
2. **测试复杂任务**：验证主 Agent 会创建子 Agent
3. **测试进度查询**：验证能查询子 Agent 状态
4. **观察日志**：确认工具调用和子 Agent 执行

---

**准备就绪！** 现在可以在 Web 界面测试管理模式了。

建议从简单任务开始，然后逐步测试复杂任务，观察主 Agent 的行为是否符合预期。
