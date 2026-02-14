package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/types"
	"go.uber.org/zap"
)

// RunOptions 单次运行的可选覆盖（如子 agent 使用 agents.defaults.subagents 的 model/max_iterations）
type RunOptions struct {
	Model          string // 覆盖本次调用的模型，空表示用 config.Model
	MaxIterations  int    // 覆盖本次最大迭代数，<=0 表示用 config.MaxIterations
}

// Orchestrator manages the agent execution loop
// Based on pi-mono's agent-loop.ts design
type Orchestrator struct {
	config          *LoopConfig
	state           *AgentState
	eventChan       chan *Event
	cancelFunc      context.CancelFunc
	progressTracker *ProgressTracker
	runOpts         *RunOptions   // 本次 Run 的覆盖，仅 Run 内有效
	lastLLMCallTime time.Time     // 上次调用 LLM 的时间，用于 model_request_interval 间隔
}

// NewOrchestrator creates a new agent orchestrator
func NewOrchestrator(config *LoopConfig, initialState *AgentState) *Orchestrator {
	// 事件通道缓冲需足够大，避免流式输出时消费者稍慢导致 orchestrator 阻塞
	return &Orchestrator{
		config:          config,
		state:           initialState,
		eventChan:       make(chan *Event, 512),
		progressTracker: NewProgressTracker(initialState.SessionKey),
	}
}

// Run starts the agent loop with initial prompts. opts is optional (e.g. subagent run uses subagents model/max_iterations).
func (o *Orchestrator) Run(ctx context.Context, prompts []AgentMessage, opts *RunOptions) ([]AgentMessage, error) {
	o.runOpts = opts
	defer func() { o.runOpts = nil }()

	logger.Info("=== Orchestrator Run Start ===",
		zap.Int("prompts_count", len(prompts)))

	ctx, cancel := context.WithCancel(ctx)
	o.cancelFunc = cancel

	// Initialize state with prompts
	newMessages := make([]AgentMessage, len(prompts))
	copy(newMessages, prompts)
	currentState := o.state.Clone()
	currentState.AddMessages(newMessages)

	// Start progress tracking（子 agent 可通过 opts.MaxIterations 覆盖）
	maxIter := o.effectiveMaxIterations()
	o.progressTracker.Start(maxIter)

	// Emit start event
	o.emit(NewEvent(EventAgentStart))

	// Main loop
	finalMessages, err := o.runLoop(ctx, currentState)

	logger.Info("=== Orchestrator Run End ===",
		zap.Int("final_messages_count", len(finalMessages)),
		zap.Error(err))

	// Update progress tracking
	if err != nil {
		o.progressTracker.Error(err)
	} else {
		o.progressTracker.Complete()
	}

	// Emit end event
	endEvent := NewEvent(EventAgentEnd)
	if finalMessages != nil {
		endEvent = NewEvent(EventAgentEnd).WithFinalMessages(finalMessages)
	}
	o.emit(endEvent)

	cancel()
	if err != nil {
		return nil, fmt.Errorf("agent loop failed: %w", err)
	}

	return finalMessages, nil
}

// effectiveMaxIterations 返回本次运行的有效最大迭代数（含 RunOptions 覆盖）
func (o *Orchestrator) effectiveMaxIterations() int {
	if o.runOpts != nil && o.runOpts.MaxIterations > 0 {
		return o.runOpts.MaxIterations
	}
	maxIter := o.config.MaxIterations
	if maxIter <= 0 {
		return 15
	}
	return maxIter
}

// effectiveModel 返回本次运行的有效模型（子 agent 可通过 RunOptions.Model 覆盖）
func (o *Orchestrator) effectiveModel() string {
	if o.runOpts != nil && strings.TrimSpace(o.runOpts.Model) != "" {
		return strings.TrimSpace(o.runOpts.Model)
	}
	return strings.TrimSpace(o.config.Model)
}

// runLoop implements the main agent loop logic
func (o *Orchestrator) runLoop(ctx context.Context, state *AgentState) ([]AgentMessage, error) {
	firstTurn := true

	// Check for steering messages at start
	pendingMessages := o.fetchSteeringMessages()

	maxIter := o.effectiveMaxIterations()
	iteration := 0

	// Outer loop: continues when queued follow-up messages arrive
	for {
		hasMoreToolCalls := true
		steeringAfterTools := false

		// Inner loop: process tool calls and steering messages
		for hasMoreToolCalls || len(pendingMessages) > 0 {
			if ctx.Err() != nil {
				return state.Messages, ctx.Err()
			}
			iteration++
			if iteration > maxIter {
				logger.Warn("Max iterations reached, stopping to prevent infinite loop",
					zap.Int("max_iterations", maxIter))
				return state.Messages, fmt.Errorf("max iterations (%d) reached, stopping to prevent infinite loop", maxIter)
			}

			if !firstTurn {
				o.emit(NewEvent(EventTurnStart))
			} else {
				firstTurn = false
			}

			// Update progress
			o.progressTracker.UpdateStep(fmt.Sprintf("Turn %d/%d", iteration, maxIter))
			o.progressTracker.CompleteStep()

			// Process pending messages (inject before next assistant response)
			if len(pendingMessages) > 0 {
				for _, msg := range pendingMessages {
					o.emit(NewEvent(EventMessageStart))
					state.AddMessage(msg)
					o.emit(NewEvent(EventMessageEnd))
				}
				pendingMessages = []AgentMessage{}
			}

			// Stream assistant response（上下文溢出时先截断历史重试，再可选做 LLM 摘要压缩后重试，与 OpenClaw 方案 B 对齐）
			var assistantMsg AgentMessage
			var err error
			const maxContextOverflowRetries = 2 // 0=正常 1=截断 2=摘要压缩
			retryTurns := 5
			contextWindow := o.config.ContextWindowTokens
			if contextWindow <= 0 {
				contextWindow = DefaultContextWindowTokens
			}
			reserve := EffectiveReserveTokens(o.config.ReserveTokens)
			summarizeFunc := func(ctx context.Context, prompt string) (string, error) {
				msgs := []providers.Message{
					{Role: "system", Content: "You are a summarizer. Output a concise summary of the following conversation. Preserve key decisions, TODOs, and constraints. Output only the summary, no preamble."},
					{Role: "user", Content: prompt},
				}
				resp, callErr := o.config.Provider.Chat(ctx, msgs, nil)
				if callErr != nil {
					return "", callErr
				}
				return resp.Content, nil
			}
			for attempt := 0; attempt <= maxContextOverflowRetries; attempt++ {
				assistantMsg, err = o.streamAssistantResponseWithRateLimitRetry(ctx, state)
				if err == nil {
					break
				}
				if !IsContextOverflowError(err) {
					break
				}
				if attempt == 0 {
					logger.Warn("Context overflow, trimming history and retrying", zap.Int("attempt", 1), zap.Int("keep_turns", retryTurns))
					state.Messages = LimitHistoryTurns(state.Messages, retryTurns)
					continue
				}
				if attempt == 1 {
					compacted, compactErr := CompactWithSummary(ctx, state.Messages, contextWindow, reserve, summarizeFunc)
					if compactErr != nil {
						logger.Warn("Compaction summarization failed, giving up", zap.Error(compactErr))
						break
					}
					logger.Info("Context overflow, applied LLM summarization and retrying", zap.Int("messages_before", len(state.Messages)), zap.Int("messages_after", len(compacted)))
					state.Messages = compacted
					continue
				}
				break
			}
			if err != nil {
				o.emitErrorEnd(state, err)
				return state.Messages, err
			}

			state.AddMessage(assistantMsg)

			// Check for tool calls
			toolCalls := extractToolCalls(assistantMsg)
			hasMoreToolCalls = len(toolCalls) > 0

			if hasMoreToolCalls {
				// Update progress for tool execution
				o.progressTracker.UpdateStep(fmt.Sprintf("Executing %d tools", len(toolCalls)))

				results, steering := o.executeToolCalls(ctx, toolCalls, state)
				steeringAfterTools = len(steering) > 0

				// Add tool result messages
				for _, result := range results {
					state.AddMessage(result)
				}

				// If steering messages arrived, skip remaining tools
				if steeringAfterTools {
					pendingMessages = steering
					break
				}
			}

			o.emit(NewEvent(EventTurnEnd))

			// Get steering messages after turn completes
			if !steeringAfterTools && len(pendingMessages) == 0 {
				pendingMessages = o.fetchSteeringMessages()
			}
		}

		// Agent would stop here. Check for follow-up messages.
		if ctx.Err() != nil {
			return state.Messages, ctx.Err()
		}
		followUpMessages := o.fetchFollowUpMessages()
		if len(followUpMessages) > 0 {
			pendingMessages = append(pendingMessages, followUpMessages...)
			continue
		}

		// No more messages, exit
		break
	}

	return state.Messages, nil
}

const maxRateLimitRetries = 2 // 限流时最多额外重试次数（共 maxRateLimitRetries+1 次调用）

// streamAssistantResponseWithRateLimitRetry 调用 LLM；若返回 406/429 等限流错误则按上游提示等待后重试，避免用户看到原始 406 即失败
func (o *Orchestrator) streamAssistantResponseWithRateLimitRetry(ctx context.Context, state *AgentState) (AgentMessage, error) {
	classifier := types.NewSimpleErrorClassifier()
	var lastErr error
	for attempt := 0; attempt <= maxRateLimitRetries; attempt++ {
		// 若配置了模型请求最小间隔，则等待至满足间隔后再调用（缓解同一会话内连续请求导致 406）
		if o.config.ModelRequestInterval > 0 {
			elapsed := time.Since(o.lastLLMCallTime)
			if elapsed < o.config.ModelRequestInterval {
				wait := o.config.ModelRequestInterval - elapsed
				logger.Debug("Model request interval: waiting before next LLM call", zap.Duration("wait", wait))
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return AgentMessage{}, ctx.Err()
				}
			}
			o.lastLLMCallTime = time.Now()
		}
		msg, err := o.streamAssistantResponse(ctx, state)
		if err == nil {
			return msg, nil
		}
		lastErr = err
		if classifier.ClassifyError(err) != types.FailoverReasonRateLimit {
			return AgentMessage{}, err
		}
		delaySec := types.ExtractRateLimitDelay(err, 30, 60)
		if delaySec <= 0 {
			delaySec = 30
		}
		if attempt < maxRateLimitRetries {
			logger.Info("Rate limit / 406, waiting before retry",
				zap.Int("wait_seconds", delaySec),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRateLimitRetries),
				zap.Error(err))
			timer := time.NewTimer(time.Duration(delaySec) * time.Second)
			select {
			case <-timer.C:
				// 继续重试
			case <-ctx.Done():
				timer.Stop()
				return AgentMessage{}, ctx.Err()
			}
			timer.Stop()
		}
	}
	return AgentMessage{}, lastErr
}

// streamAssistantResponse calls the LLM and streams the response
func (o *Orchestrator) streamAssistantResponse(ctx context.Context, state *AgentState) (AgentMessage, error) {
	logger.Debug("streamAssistantResponse Start",
		zap.Int("message_count", len(state.Messages)),
		zap.Strings("loaded_skills", state.LoadedSkills))

	state.IsStreaming = true
	defer func() { state.IsStreaming = false }()

	// Apply context transform if configured
	messages := state.Messages
	if o.config.TransformContext != nil {
		transformed, err := o.config.TransformContext(messages)
		if err == nil {
			messages = transformed
		} else {
			logger.Warn("Context transform failed, using original", zap.Error(err))
		}
	}

	// Context window: trim history and truncate tool results before sending to LLM
	contextWindow := o.config.ContextWindowTokens
	if contextWindow <= 0 {
		contextWindow = DefaultContextWindowTokens
	}
	reserve := EffectiveReserveTokens(o.config.ReserveTokens)
	maxTurns := o.config.MaxHistoryTurns
	if maxTurns > 0 {
		messages = LimitHistoryTurns(messages, maxTurns)
		logger.Debug("Context: limited history turns", zap.Int("max_turns", maxTurns), zap.Int("messages_after", len(messages)))
	}
	messages = CopyMessagesWithTruncatedToolResults(messages, contextWindow)
	if limit := contextWindow - reserve; limit > 0 {
		estimated := EstimateMessagesTokens(messages)
		if estimated > limit {
			logger.Warn("Context: estimated tokens may exceed window", zap.Int("estimated", estimated), zap.Int("limit", limit))
		}
	}

	// Convert to provider messages
	var providerMsgs []providers.Message
	if o.config.ConvertToLLM != nil {
		converted, err := o.config.ConvertToLLM(messages)
		if err != nil {
			return AgentMessage{}, fmt.Errorf("convert to LLM failed: %w", err)
		}
		providerMsgs = converted
	} else {
		// Default conversion
		providerMsgs = convertToProviderMessages(messages)
	}

	// Prepare tool definitions
	toolDefs := convertToToolDefinitions(state.Tools)

	// Emit message start
	o.emit(NewEvent(EventMessageStart))

	// Call provider with system prompt as first message
	fullMessages := []providers.Message{}

	// Build system prompt with skills if context builder is available
	if o.config.ContextBuilder != nil {
		skillsContent := ""
		if len(state.LoadedSkills) > 0 {
			// Second phase: inject full content of loaded skills
			skillsContent = o.config.ContextBuilder.buildSelectedSkills(state.LoadedSkills, o.config.Skills)
		} else if len(o.config.Skills) > 0 {
			// First phase: inject skill summary (available skills list)
			skillsContent = o.config.ContextBuilder.buildSkillsPrompt(o.config.Skills, PromptModeFull)
		}
		systemPrompt := o.config.ContextBuilder.buildSystemPromptWithSkills(skillsContent, PromptModeFull)
		fullMessages = append(fullMessages, providers.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	} else if state.SystemPrompt != "" {
		// Fallback to stored system prompt
		fullMessages = append(fullMessages, providers.Message{
			Role:    "system",
			Content: state.SystemPrompt,
		})
	}
	fullMessages = append(fullMessages, providerMsgs...)

	// 本次运行的有效 model（子 agent 可通过 RunOptions.Model 覆盖）
	modelForRequest := o.effectiveModel()
	if modelForRequest == "" || strings.EqualFold(modelForRequest, "default") {
		modelForRequest = "(provider default)"
	}
	logger.Info("=== Calling LLM ===",
		zap.String("model", modelForRequest),
		zap.Int("messages_count", len(fullMessages)),
		zap.Int("tools_count", len(toolDefs)),
		zap.Bool("has_loaded_skills", len(state.LoadedSkills) > 0))

	// 从配置组装 LLM 调用选项
	chatOpts := []providers.ChatOption{}
	if model := strings.TrimSpace(o.effectiveModel()); model != "" && !strings.EqualFold(model, "default") {
		chatOpts = append(chatOpts, providers.WithModel(model))
	}
	if o.config.Temperature > 0 {
		chatOpts = append(chatOpts, providers.WithTemperature(o.config.Temperature))
	}
	if o.config.MaxTokens > 0 {
		chatOpts = append(chatOpts, providers.WithMaxTokens(o.config.MaxTokens))
	}

	// 检查是否支持流式输出（provider 支持且实现了 StreamingProvider 接口）
	streamingProvider, supportsStreaming := o.config.Provider.(providers.StreamingProvider)
	useStreaming := supportsStreaming && o.config.Provider.SupportsStreaming()

	var response *providers.Response
	var err error

	if useStreaming {
		// 使用流式 API
		logger.Debug("Using streaming API")
		var content strings.Builder
		var toolCalls []providers.ToolCall

		err = streamingProvider.ChatStream(ctx, fullMessages, toolDefs, func(chunk providers.StreamChunk) {
			if chunk.Error != nil {
				logger.Error("Stream chunk error", zap.Error(chunk.Error))
				return
			}

			// 发送流式内容事件
			if chunk.Content != "" && !chunk.Done {
				o.emit(NewEvent(EventMessageDelta).WithContent(chunk.Content))
			}

			// 完成时收集工具调用
			if chunk.Done {
				content.WriteString(chunk.Content)
				toolCalls = chunk.ToolCalls
			}
		}, chatOpts...)

		if err != nil {
			logger.Error("LLM streaming call failed", zap.Error(err))
			return AgentMessage{}, fmt.Errorf("LLM streaming call failed: %w", err)
		}

		// 构建响应
		response = &providers.Response{
			Content:      content.String(),
			ToolCalls:    toolCalls,
			FinishReason: "stop",
		}
	} else {
		// 使用非流式 API
		logger.Debug("Using non-streaming API")
		response, err = o.config.Provider.Chat(ctx, fullMessages, toolDefs, chatOpts...)
		if err != nil {
			logger.Error("LLM call failed", zap.Error(err))
			return AgentMessage{}, fmt.Errorf("LLM call failed: %w", err)
		}
	}

	logger.Info("=== LLM Response Received ===",
		zap.Int("content_length", len(response.Content)),
		zap.Int("tool_calls_count", len(response.ToolCalls)),
		zap.String("content_preview", truncateString(response.Content, 200)))

	// Emit message end
	o.emit(NewEvent(EventMessageEnd))

	// Convert response to agent message
	assistantMsg := convertFromProviderResponse(response)

	logger.Debug("streamAssistantResponse End",
		zap.Bool("has_tool_calls", len(response.ToolCalls) > 0),
		zap.Int("tool_calls_count", len(response.ToolCalls)))

	return assistantMsg, nil
}

// executeToolCalls executes tool calls with interruption support
// Tools are executed in parallel for better performance
func (o *Orchestrator) executeToolCalls(ctx context.Context, toolCalls []ToolCallContent, state *AgentState) ([]AgentMessage, []AgentMessage) {
	logger.Info("=== Execute Tool Calls Start ===",
		zap.Int("count", len(toolCalls)))

	if len(toolCalls) == 0 {
		return nil, nil
	}

	// Structure to hold tool execution results with order
	type toolExecutionResult struct {
		index      int
		toolCall   ToolCallContent
		result     ToolResult
		err        error
		resultMsg  AgentMessage
		skillName  string // For use_skill tracking
	}

	// Execute tools in parallel
	var wg sync.WaitGroup
	resultsChan := make(chan toolExecutionResult, len(toolCalls))

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(index int, tc ToolCallContent) {
			defer wg.Done()

			logger.Debug("Tool call start",
				zap.String("tool_id", tc.ID),
				zap.String("tool_name", tc.Name),
				zap.Any("arguments", tc.Arguments))

			// Update progress tracking
			o.progressTracker.StartTool(tc.Name, len(toolCalls))

			// Emit tool execution start
			o.emit(NewEvent(EventToolExecutionStart).WithToolExecution(tc.ID, tc.Name, tc.Arguments))

			// Find tool
			var tool Tool
			for _, t := range state.Tools {
				if t.Name() == tc.Name {
					tool = t
					break
				}
			}

			var result ToolResult
			var err error
			var skillName string

			if tool == nil {
				err = fmt.Errorf("tool %s not found", tc.Name)
				result = ToolResult{
					Content: []ContentBlock{TextContent{Text: fmt.Sprintf("Tool not found: %s", tc.Name)}},
					Details: map[string]any{"error": err.Error()},
				}
				logger.Error("Tool not found",
					zap.String("tool_name", tc.Name),
					zap.String("tool_id", tc.ID))
			} else {
				state.AddPendingTool(tc.ID)

				// 将 session key 添加到 context 中，供工具使用
				toolCtx := context.WithValue(ctx, "session_key", state.SessionKey)

				// Execute tool with streaming support
				result, err = tool.Execute(toolCtx, tc.Arguments, func(partial ToolResult) {
					// Emit update event
					o.emit(NewEvent(EventToolExecutionUpdate).
						WithToolExecution(tc.ID, tc.Name, tc.Arguments).
						WithToolResult(&partial, false))
				})

				state.RemovePendingTool(tc.ID)

				// Check for use_skill
				if tc.Name == "use_skill" && err == nil {
					if sn, ok := tc.Arguments["skill_name"].(string); ok && sn != "" {
						skillName = sn
					}
				}
			}

			// Log tool execution result
			if err != nil {
				logger.Error("Tool execution failed",
					zap.String("tool_id", tc.ID),
					zap.String("tool_name", tc.Name),
					zap.Any("arguments", tc.Arguments),
					zap.Error(err))
			} else {
				// Extract content for logging
				contentText := extractToolResultContent(result.Content)
				logger.Debug("Tool execution success",
					zap.String("tool_id", tc.ID),
					zap.String("tool_name", tc.Name),
					zap.Any("arguments", tc.Arguments),
					zap.Int("result_length", len(contentText)),
					zap.String("result_preview", truncateString(contentText, 200)))
			}

			// Convert result to message
			resultMsg := AgentMessage{
				Role:      RoleToolResult,
				Content:   result.Content,
				Timestamp: time.Now().UnixMilli(),
				Metadata:  map[string]any{"tool_call_id": tc.ID, "tool_name": tc.Name},
			}

			if err != nil {
				resultMsg.Metadata["error"] = err.Error()
				result.Content = []ContentBlock{TextContent{Text: err.Error()}}
			}

			// Update progress tracking
			o.progressTracker.CompleteTool(tc.Name)

			// Emit tool execution end
			event := NewEvent(EventToolExecutionEnd).
				WithToolExecution(tc.ID, tc.Name, tc.Arguments).
				WithToolResult(&result, err != nil)
			o.emit(event)

			// Send result to channel
			resultsChan <- toolExecutionResult{
				index:     index,
				toolCall:  tc,
				result:    result,
				err:       err,
				resultMsg: resultMsg,
				skillName: skillName,
			}
		}(i, tc)
	}

	// Wait for all tools to complete
	wg.Wait()
	close(resultsChan)

	// Collect results in order
	resultsMap := make(map[int]toolExecutionResult)
	for res := range resultsChan {
		resultsMap[res.index] = res
	}

	// Build ordered results
	results := make([]AgentMessage, 0, len(toolCalls))
	for i := 0; i < len(toolCalls); i++ {
		res := resultsMap[i]
		results = append(results, res.resultMsg)

		// Update LoadedSkills for use_skill
		if res.skillName != "" {
			alreadyLoaded := false
			for _, loaded := range state.LoadedSkills {
				if loaded == res.skillName {
					alreadyLoaded = true
					break
				}
			}
			if !alreadyLoaded {
				state.LoadedSkills = append(state.LoadedSkills, res.skillName)
				logger.Info("=== Skill Loaded ===",
					zap.String("skill_name", res.skillName),
					zap.Int("total_loaded", len(state.LoadedSkills)),
					zap.Strings("loaded_skills", state.LoadedSkills))
			}
		}
	}

	// Check for steering messages (interruption) after all tools complete
	steering := o.fetchSteeringMessages()
	if len(steering) > 0 {
		return results, steering
	}

	logger.Info("=== Execute Tool Calls End ===",
		zap.Int("count", len(results)))
	return results, nil
}

// emit sends an event to the event channel (non-blocking to avoid deadlock when consumer is slow)
func (o *Orchestrator) emit(event *Event) {
	if o.eventChan == nil {
		return
	}
	select {
	case o.eventChan <- event:
	default:
		logger.Debug("event channel full, dropping event", zap.String("type", string(event.Type)))
	}
}

// emitErrorEnd emits an error end event
func (o *Orchestrator) emitErrorEnd(state *AgentState, err error) {
	event := NewEvent(EventTurnEnd).WithStopReason(err.Error())
	o.emit(event)
}

// fetchSteeringMessages gets steering messages from config
func (o *Orchestrator) fetchSteeringMessages() []AgentMessage {
	if o.config.GetSteeringMessages != nil {
		msgs, _ := o.config.GetSteeringMessages()
		return msgs
	}
	// Fall back to state queue
	return o.state.DequeueSteeringMessages()
}

// fetchFollowUpMessages gets follow-up messages from config
func (o *Orchestrator) fetchFollowUpMessages() []AgentMessage {
	if o.config.GetFollowUpMessages != nil {
		msgs, _ := o.config.GetFollowUpMessages()
		return msgs
	}
	// Fall back to state queue
	return o.state.DequeueFollowUpMessages()
}

// Stop stops the orchestrator
func (o *Orchestrator) Stop() {
	if o.cancelFunc != nil {
		o.cancelFunc()
	}
	if o.eventChan != nil {
		close(o.eventChan)
	}
}

// Subscribe returns the event channel
func (o *Orchestrator) Subscribe() <-chan *Event {
	return o.eventChan
}

// GetProgressTracker returns the progress tracker
func (o *Orchestrator) GetProgressTracker() *ProgressTracker {
	return o.progressTracker
}

// SubscribeProgress subscribes to progress updates
func (o *Orchestrator) SubscribeProgress() <-chan *ProgressUpdate {
	return o.progressTracker.Subscribe()
}

// Helper functions

// convertToProviderMessages converts agent messages to provider messages
func convertToProviderMessages(messages []AgentMessage) []providers.Message {
	result := make([]providers.Message, 0, len(messages))

	for _, msg := range messages {
		// Skip system messages
		if msg.Role == RoleSystem {
			continue
		}

		providerMsg := providers.Message{
			Role: string(msg.Role),
		}
		if reasoning, ok := msg.Metadata["reasoning_content"].(string); ok {
			providerMsg.ReasoningContent = reasoning
		}

		// Extract content
		for _, block := range msg.Content {
			switch b := block.(type) {
			case TextContent:
				if providerMsg.Content != "" {
					providerMsg.Content += "\n" + b.Text
				} else {
					providerMsg.Content = b.Text
				}
			case ImageContent:
				if b.Data != "" {
					providerMsg.Images = append(providerMsg.Images, b.Data)
				} else if b.URL != "" {
					providerMsg.Images = append(providerMsg.Images, b.URL)
				}
			case ThinkingContent:
				if strings.TrimSpace(providerMsg.ReasoningContent) == "" {
					providerMsg.ReasoningContent = b.Thinking
				}
			}
		}

		// Handle tool calls for assistant messages
		if msg.Role == RoleAssistant {
			var toolCalls []providers.ToolCall
			for _, block := range msg.Content {
				if tc, ok := block.(ToolCallContent); ok {
					toolCalls = append(toolCalls, providers.ToolCall{
						ID:     tc.ID,
						Name:   tc.Name,
						Params: convertMapAnyToInterface(tc.Arguments),
					})
				}
			}
			providerMsg.ToolCalls = toolCalls
		}

		// Handle tool_call_id and tool_name for tool result messages
		if msg.Role == RoleToolResult {
			if toolCallID, ok := msg.Metadata["tool_call_id"].(string); ok {
				providerMsg.ToolCallID = toolCallID
			}
			if toolName, ok := msg.Metadata["tool_name"].(string); ok {
				providerMsg.ToolName = toolName
			}
		}

		result = append(result, providerMsg)
	}

	return result
}

// convertFromProviderResponse converts provider response to agent message
func convertFromProviderResponse(response *providers.Response) AgentMessage {
	content := []ContentBlock{TextContent{Text: response.Content}}

	// Handle tool calls
	for _, tc := range response.ToolCalls {
		content = append(content, ToolCallContent{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: convertInterfaceToAny(tc.Params),
		})
	}

	metadata := map[string]any{"stop_reason": response.FinishReason}
	if strings.TrimSpace(response.ReasoningContent) != "" {
		metadata["reasoning_content"] = response.ReasoningContent
	}

	return AgentMessage{
		Role:      RoleAssistant,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  metadata,
	}
}

// convertToToolDefinitions converts agent tools to provider tool definitions
func convertToToolDefinitions(tools []Tool) []providers.ToolDefinition {
	result := make([]providers.ToolDefinition, 0, len(tools))

	for _, tool := range tools {
		result = append(result, providers.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  convertMapAnyToInterface(tool.Parameters()),
		})
	}

	return result
}

// extractToolCalls extracts tool calls from a message
func extractToolCalls(msg AgentMessage) []ToolCallContent {
	var toolCalls []ToolCallContent

	for _, block := range msg.Content {
		if tc, ok := block.(ToolCallContent); ok {
			toolCalls = append(toolCalls, tc)
		}
	}

	return toolCalls
}

// convertInterfaceToAny converts map[string]interface{} to map[string]any
func convertInterfaceToAny(m map[string]interface{}) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		result[k] = v
	}
	return result
}

// extractToolResultContent extracts text content from tool result
func extractToolResultContent(content []ContentBlock) string {
	var result strings.Builder
	for _, block := range content {
		if text, ok := block.(TextContent); ok {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(text.Text)
		}
	}
	return result.String()
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen > 3 {
		return s[:maxLen-3] + "..."
	}
	return s[:maxLen]
}
