package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// OpenAIProvider OpenAI provider
type OpenAIProvider struct {
	client            openai.Client
	model             string
	baseURL           string
	maxTokens         int
	extraBody         map[string]interface{}
	streamingEnabled  bool // 是否启用流式输出
	router9Compatible bool // 9router 兼容模式
	skipTools         bool // 9router 下为 true 时不传 tools，用于排查 406
}

// NewOpenAIProvider creates an OpenAI provider.
func NewOpenAIProvider(apiKey, baseURL, model string, maxTokens int, extraBody map[string]interface{}) (*OpenAIProvider, error) {
	return NewOpenAIProviderWithStreaming(apiKey, baseURL, model, maxTokens, extraBody, true)
}

// NewOpenAIProviderWithStreaming creates an OpenAI provider with streaming configuration.
// 若 optSkipTools 传 true（仅 9router 配置 tools_enabled: false 时），请求时不带 tools，用于排查 406。
func NewOpenAIProviderWithStreaming(apiKey, baseURL, model string, maxTokens int, extraBody map[string]interface{}, streaming bool, optSkipTools ...bool) (*OpenAIProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if model == "" {
		model = "gpt-4"
	}

	clientOpts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(baseURL))
	}

	// 自动检测 9router 代理
	router9Compatible := strings.Contains(baseURL, "localhost:20128") ||
		strings.Contains(baseURL, "127.0.0.1:20128") ||
		strings.Contains(baseURL, ":20128")

	skipTools := false
	if len(optSkipTools) > 0 {
		skipTools = optSkipTools[0]
	}

	if router9Compatible {
		logger.Debug("Detected 9router proxy, enabling compatibility mode",
			zap.String("base_url", baseURL),
			zap.Bool("skip_tools", skipTools))
	}

	return &OpenAIProvider{
		client:            openai.NewClient(clientOpts...),
		model:             model,
		baseURL:           baseURL,
		maxTokens:         maxTokens,
		extraBody:         copyExtraBody(extraBody),
		streamingEnabled:  streaming,
		router9Compatible: router9Compatible,
		skipTools:         skipTools,
	}, nil
}

// SupportsStreaming returns whether streaming is enabled for this provider.
func (p *OpenAIProvider) SupportsStreaming() bool {
	return p.streamingEnabled
}

// Chat performs a chat completion request.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	opts := &ChatOptions{
		Model:       p.model,
		Temperature: 0,
		MaxTokens:   p.maxTokens,
		Stream:      false,
	}

	for _, opt := range options {
		opt(opts)
	}

	openAIMessages, err := convertMessagesToOpenAI(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	req := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(opts.Model),
		Messages: openAIMessages,
	}
	// 仅当配置里显式设置了 temperature 时才传，不硬编码
	if opts.Temperature > 0 {
		req.Temperature = openai.Float(opts.Temperature)
	}
	if opts.MaxTokens > 0 {
		req.MaxTokens = openai.Int(int64(opts.MaxTokens))
	}
	toolsToSend := tools
	if p.skipTools {
		toolsToSend = nil
	}
	if len(toolsToSend) > 0 {
		req.Tools = convertToolsToOpenAI(toolsToSend)
	}

	// 9router 兼容模式：禁用 reasoning_content 和部分 extra_body 参数
	var reqOpts []option.RequestOption
	if p.router9Compatible {
		// 9router 兼容模式：不添加任何 extra_body 和 reasoning_content
		reqOpts = []option.RequestOption{}
		logger.Info("9router non-streaming request",
			zap.String("model", opts.Model),
			zap.Int("messages", len(openAIMessages)),
			zap.Int("tools_sent", len(toolsToSend)),
			zap.Bool("has_temperature", opts.Temperature > 0),
			zap.Bool("has_max_tokens", opts.MaxTokens > 0),
			zap.Float64("temperature_value", opts.Temperature),
			zap.Int("max_tokens_value", opts.MaxTokens))
	} else {
		reqOpts = append(p.extraBodyOptions(), assistantReasoningOptions(messages)...)
	}

	completion, err := p.client.Chat.Completions.New(ctx, req, reqOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if completion == nil || len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from model")
	}

	choice := completion.Choices[0]
	response := &Response{
		Content:          choice.Message.Content,
		ReasoningContent: extractReasoningContent(choice.Message),
		ToolCalls:        parseOpenAIToolCalls(choice.Message.ToolCalls),
		FinishReason:     choice.FinishReason,
		Usage: Usage{
			PromptTokens:     int(completion.Usage.PromptTokens),
			CompletionTokens: int(completion.Usage.CompletionTokens),
			TotalTokens:      int(completion.Usage.TotalTokens),
		},
	}
	if response.FinishReason == "" {
		response.FinishReason = "stop"
	}

	return response, nil
}

// ChatWithTools chats with tool definitions.
func (p *OpenAIProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	return p.Chat(ctx, messages, tools, options...)
}

// Close closes provider resources.
func (p *OpenAIProvider) Close() error {
	return nil
}

// NewOpenAIProviderFromLangChain keeps backward compatibility for older call sites.
func NewOpenAIProviderFromLangChain(apiKey, baseURL, model string, maxTokens int) (Provider, error) {
	return NewOpenAIProvider(apiKey, baseURL, model, maxTokens, nil)
}

func convertMessagesToOpenAI(messages []Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		converted, err := convertMessageToOpenAI(msg)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func convertMessageToOpenAI(msg Message) (openai.ChatCompletionMessageParamUnion, error) {
	switch msg.Role {
	case "system":
		return openai.SystemMessage(msg.Content), nil
	case "user":
		if len(msg.Images) == 0 {
			return openai.UserMessage(msg.Content), nil
		}

		parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.Images)+1)
		if msg.Content != "" {
			parts = append(parts, openai.TextContentPart(msg.Content))
		}
		for _, img := range msg.Images {
			if img == "" {
				continue
			}
			parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL: img,
			}))
		}
		if len(parts) == 0 {
			parts = append(parts, openai.TextContentPart(""))
		}
		return openai.UserMessage(parts), nil
	case "assistant":
		return convertAssistantMessageToOpenAI(msg), nil
	case "tool":
		if msg.ToolCallID == "" {
			return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("tool message is missing tool_call_id")
		}
		return openai.ToolMessage(msg.Content, msg.ToolCallID), nil
	default:
		return openai.UserMessage(msg.Content), nil
	}
}

func convertAssistantMessageToOpenAI(msg Message) openai.ChatCompletionMessageParamUnion {
	if len(msg.ToolCalls) == 0 {
		return openai.AssistantMessage(msg.Content)
	}

	assistant := openai.ChatCompletionAssistantMessageParam{}
	if msg.Content != "" {
		assistant.Content.OfString = openai.String(msg.Content)
	}

	assistant.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, 0, len(msg.ToolCalls))
	for i, tc := range msg.ToolCalls {
		rawArgs := "{}"
		if len(tc.Params) > 0 {
			args, err := json.Marshal(tc.Params)
			if err != nil {
				logger.Error("Failed to marshal assistant tool call arguments",
					zap.String("tool", tc.Name),
					zap.String("id", tc.ID),
					zap.Error(err))
			} else {
				rawArgs = string(args)
			}
		}

		toolCallID := tc.ID
		if toolCallID == "" {
			toolCallID = fmt.Sprintf("call_%d", i)
		}

		assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallParam{
			ID: toolCallID,
			Function: openai.ChatCompletionMessageToolCallFunctionParam{
				Name:      tc.Name,
				Arguments: rawArgs,
			},
		})
	}

	return openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant}
}

func convertToolsToOpenAI(tools []ToolDefinition) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, tool := range tools {
		function := shared.FunctionDefinitionParam{
			Name: tool.Name,
		}
		if tool.Description != "" {
			function.Description = openai.String(tool.Description)
		}
		if len(tool.Parameters) > 0 {
			function.Parameters = shared.FunctionParameters(tool.Parameters)
		}

		result = append(result, openai.ChatCompletionToolParam{
			Function: function,
		})
	}
	return result
}

func parseOpenAIToolCalls(toolCalls []openai.ChatCompletionMessageToolCall) []ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	result := make([]ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		var params map[string]interface{}
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
				logger.Error("Failed to unmarshal tool arguments",
					zap.String("tool", tc.Function.Name),
					zap.String("id", tc.ID),
					zap.Error(err),
					zap.String("raw_args", tc.Function.Arguments),
					zap.Int("args_length", len(tc.Function.Arguments)))

				params = map[string]interface{}{
					"__error__":         fmt.Sprintf("Failed to parse arguments: %v", err),
					"__raw_arguments__": tc.Function.Arguments,
				}
			}
		}
		if params == nil {
			params = map[string]interface{}{}
		}

		result = append(result, ToolCall{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Params: params,
		})
	}

	return result
}

// assistantReasoningOptions 为每条 assistant 消息设置 reasoning_content。
// Moonshot/Kimi 在开启 thinking 时要求带 tool_calls 的 assistant 消息必须包含该字段，缺则 400。
// 有内容用内容，无内容用空字符串，避免 "reasoning_content is missing"。
func assistantReasoningOptions(messages []Message) []option.RequestOption {
	if len(messages) == 0 {
		return nil
	}
	opts := make([]option.RequestOption, 0)
	for i, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		reasoning := strings.TrimSpace(msg.ReasoningContent)
		// 始终设置 reasoning_content，无则传 ""，满足 Moonshot thinking 对 tool call 消息的要求
		opts = append(opts, option.WithJSONSet(fmt.Sprintf("messages.%d.reasoning_content", i), reasoning))
	}
	return opts
}

func extractReasoningContent(msg openai.ChatCompletionMessage) string {
	raw := strings.TrimSpace(msg.RawJSON())
	if raw == "" {
		return ""
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}

	value, ok := payload["reasoning_content"]
	if !ok || value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			switch s := item.(type) {
			case string:
				if strings.TrimSpace(s) != "" {
					parts = append(parts, s)
				}
			default:
				if b, err := json.Marshal(item); err == nil {
					parts = append(parts, string(b))
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return ""
	}
}

func copyExtraBody(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func (p *OpenAIProvider) extraBodyOptions() []option.RequestOption {
	if len(p.extraBody) == 0 {
		return nil
	}

	opts := make([]option.RequestOption, 0, len(p.extraBody))
	for key, value := range p.extraBody {
		if strings.TrimSpace(key) == "" {
			continue
		}
		opts = append(opts, option.WithJSONSet(key, value))
	}
	return opts
}

// ChatStream performs a streaming chat completion request.
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, callback StreamCallback, options ...ChatOption) error {
	opts := &ChatOptions{
		Model:       p.model,
		Temperature: 0,
		MaxTokens:   p.maxTokens,
		Stream:      true,
	}

	for _, opt := range options {
		opt(opts)
	}

	openAIMessages, err := convertMessagesToOpenAI(messages)
	if err != nil {
		return fmt.Errorf("failed to convert messages: %w", err)
	}

	req := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(opts.Model),
		Messages: openAIMessages,
	}
	// 仅当配置里显式设置了 temperature 时才传，不硬编码
	if opts.Temperature > 0 {
		req.Temperature = openai.Float(opts.Temperature)
	}
	if opts.MaxTokens > 0 {
		req.MaxTokens = openai.Int(int64(opts.MaxTokens))
	}	
	toolsToSend := tools
	if p.skipTools {
		toolsToSend = nil
	}
	if len(toolsToSend) > 0 {
		req.Tools = convertToolsToOpenAI(toolsToSend)
	}

	// 9router 兼容模式：禁用 reasoning_content 和部分 extra_body 参数
	var reqOpts []option.RequestOption
	if p.router9Compatible {
		// 9router 兼容模式：不添加任何 extra_body 和 reasoning_content
		reqOpts = []option.RequestOption{}
		logger.Info("9router streaming request",
			zap.String("model", opts.Model),
			zap.Int("messages", len(openAIMessages)),
			zap.Int("tools_sent", len(toolsToSend)),
			zap.Bool("has_temperature", opts.Temperature > 0),
			zap.Bool("has_max_tokens", opts.MaxTokens > 0),
			zap.Float64("temperature_value", opts.Temperature),
			zap.Int("max_tokens_value", opts.MaxTokens))

		// 详细记录每条消息的角色
		for i, msg := range openAIMessages {
			var role string
			if msg.OfSystem != nil {
				role = "system"
			} else if msg.OfUser != nil {
				role = "user"
			} else if msg.OfAssistant != nil {
				role = "assistant"
			} else if msg.OfTool != nil {
				role = "tool"
			} else {
				role = "unknown"
			}
			logger.Debug("Message in request", zap.Int("index", i), zap.String("role", role))
		}
	} else {
		reqOpts = append(p.extraBodyOptions(), assistantReasoningOptions(messages)...)
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, req, reqOpts...)

	// 累积工具调用
	toolCallsMap := make(map[int]*ToolCall)
	var content strings.Builder

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// 处理文本内容
		if delta.Content != "" {
			content.WriteString(delta.Content)
			callback(StreamChunk{
				Content: delta.Content,
				Done:    false,
			})
		}

		// 处理工具调用
		for _, tc := range delta.ToolCalls {
			idx := int(tc.Index)
			if _, exists := toolCallsMap[idx]; !exists {
				toolCallsMap[idx] = &ToolCall{
					ID:   tc.ID,
					Name: tc.Function.Name,
				}
			}
			// 累积参数
			if tc.Function.Arguments != "" {
				existing := toolCallsMap[idx]
				if existing.rawArgs == "" {
					existing.rawArgs = tc.Function.Arguments
				} else {
					existing.rawArgs += tc.Function.Arguments
				}
			}
		}

		// 检查是否完成
		if chunk.Choices[0].FinishReason != "" {
			break
		}
	}

	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	// 解析工具调用参数
	var toolCalls []ToolCall
	for i := 0; i < len(toolCallsMap); i++ {
		tc := toolCallsMap[i]
		if tc != nil {
			if tc.rawArgs != "" {
				var params map[string]interface{}
				if err := json.Unmarshal([]byte(tc.rawArgs), &params); err != nil {
					logger.Error("Failed to unmarshal streaming tool arguments",
						zap.String("tool", tc.Name),
						zap.Error(err))
					params = map[string]interface{}{}
				}
				tc.Params = params
			}
			toolCalls = append(toolCalls, *tc)
		}
	}

	// 发送完成信号
	callback(StreamChunk{
		Content:   content.String(),
		Done:      true,
		ToolCalls: toolCalls,
	})

	return nil
}
