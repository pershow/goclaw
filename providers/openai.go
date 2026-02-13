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
	client    openai.Client
	model     string
	maxTokens int
	extraBody map[string]interface{}
}

// NewOpenAIProvider creates an OpenAI provider.
func NewOpenAIProvider(apiKey, baseURL, model string, maxTokens int, extraBody map[string]interface{}) (*OpenAIProvider, error) {
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

	return &OpenAIProvider{
		client:    openai.NewClient(clientOpts...),
		model:     model,
		maxTokens: maxTokens,
		extraBody: copyExtraBody(extraBody),
	}, nil
}

// Chat performs a chat completion request.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	opts := &ChatOptions{
		Model:       p.model,
		Temperature: 0.7,
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
	if opts.Temperature > 0 {
		req.Temperature = openai.Float(opts.Temperature)
	}
	if opts.MaxTokens > 0 {
		req.MaxTokens = openai.Int(int64(opts.MaxTokens))
	}
	if len(tools) > 0 {
		req.Tools = convertToolsToOpenAI(tools)
	}

	completion, err := p.client.Chat.Completions.New(ctx, req, p.extraBodyOptions()...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if completion == nil || len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from model")
	}

	choice := completion.Choices[0]
	response := &Response{
		Content:      choice.Message.Content,
		ToolCalls:    parseOpenAIToolCalls(choice.Message.ToolCalls),
		FinishReason: choice.FinishReason,
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
