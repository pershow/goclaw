package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkhttpserverext "github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"go.uber.org/zap"
)

const (
	feishuEventModeWebhook        = "webhook"
	feishuEventModeLongConnection = "long_connection"
	feishuTypingEmoji             = "Typing"
	feishuTypingTTL               = 10 * time.Minute
)

type feishuTypingReaction struct {
	messageID  string
	reactionID string
	createdAt  time.Time
}

// FeishuChannel 飞书通道
type FeishuChannel struct {
	*BaseChannelImpl
	appID             string
	appSecret         string
	encryptKey        string
	verificationToken string
	webhookPort       int
	eventMode         string
	client            *lark.Client
	dispatcher        *larkdispatcher.EventDispatcher

	mu            sync.Mutex
	webhookServer *http.Server
	wsClient      *larkws.Client
	wsCancel      context.CancelFunc
	pendingTyping map[string][]feishuTypingReaction // chatID -> queued typing reactions
}

// NewFeishuChannel 创建飞书通道
func NewFeishuChannel(cfg config.FeishuChannelConfig, bus *bus.MessageBus) (*FeishuChannel, error) {
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, fmt.Errorf("feishu app_id and app_secret are required")
	}

	eventMode, err := parseFeishuEventMode(cfg.EventMode)
	if err != nil {
		return nil, err
	}

	port := cfg.WebhookPort
	if port == 0 {
		port = 8765
	}

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	channel := &FeishuChannel{
		BaseChannelImpl:   NewBaseChannelImpl("feishu", "default", baseCfg, bus),
		appID:             cfg.AppID,
		appSecret:         cfg.AppSecret,
		encryptKey:        cfg.EncryptKey,
		verificationToken: cfg.VerificationToken,
		webhookPort:       port,
		eventMode:         eventMode,
		client:            lark.NewClient(cfg.AppID, cfg.AppSecret),
		pendingTyping:     make(map[string][]feishuTypingReaction),
	}
	channel.dispatcher = larkdispatcher.NewEventDispatcher(channel.verificationToken, channel.encryptKey).
		OnP2MessageReceiveV1(channel.handleMessageReceiveEvent)

	return channel, nil
}

func parseFeishuEventMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == feishuEventModeLongConnection {
		return mode, nil
	}
	if mode == "" || mode == feishuEventModeWebhook {
		return feishuEventModeWebhook, nil
	}
	return "", fmt.Errorf("feishu event_mode must be one of: webhook, long_connection")
}

// Start 启动飞书通道
func (c *FeishuChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Feishu channel",
		zap.String("app_id", c.appID),
		zap.String("event_mode", c.eventMode),
	)

	switch c.eventMode {
	case feishuEventModeLongConnection:
		go c.startLongConnection(ctx)
	default:
		go c.startWebhookServer(ctx)
	}

	return nil
}

func (c *FeishuChannel) startWebhookServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/feishu/webhook", larkhttpserverext.NewEventHandlerFunc(c.dispatcher))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", c.webhookPort),
		Handler: mux,
	}

	c.mu.Lock()
	c.webhookServer = server
	c.mu.Unlock()

	go func() {
		logger.Info("Feishu webhook server started", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Feishu webhook server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
		logger.Warn("Failed to shutdown Feishu webhook server", zap.Error(err))
	}
}

func (c *FeishuChannel) startLongConnection(ctx context.Context) {
	wsCtx, cancel := context.WithCancel(ctx)
	client := larkws.NewClient(c.appID, c.appSecret, larkws.WithEventHandler(c.dispatcher))

	c.mu.Lock()
	c.wsClient = client
	c.wsCancel = cancel
	c.mu.Unlock()

	logger.Info("Feishu long connection started")
	if err := client.Start(wsCtx); err != nil {
		logger.Error("Feishu long connection error", zap.Error(err))
	}
}

func (c *FeishuChannel) handleMessageReceiveEvent(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	_ = ctx

	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil {
		return nil
	}
	if !isFeishuUserSender(event.Event.Sender) {
		return nil
	}

	senderID := extractFeishuSenderID(event.Event.Sender.SenderId)
	if senderID == "" {
		return nil
	}

	if !c.IsAllowed(senderID) {
		return nil
	}

	message := event.Event.Message
	messageID := derefString(message.MessageId)
	chatID := derefString(message.ChatId)
	c.addTypingIndicator(messageID, chatID)

	msgType := derefString(message.MessageType)
	contentText := parseFeishuMessageContent(derefString(message.Content), msgType)

	msg := &bus.InboundMessage{
		ID:        messageID,
		Content:   contentText,
		AccountID: c.AccountID(),
		SenderID:  senderID,
		ChatID:    chatID,
		Channel:   c.Name(),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"chat_type": derefString(message.ChatType),
			"msg_type":  msgType,
		},
	}

	if err := c.PublishInbound(context.Background(), msg); err != nil {
		logger.Warn("Failed to publish Feishu inbound message", zap.Error(err))
	}

	return nil
}

func isFeishuUserSender(sender *larkim.EventSender) bool {
	if sender == nil {
		return false
	}
	senderType := strings.ToLower(strings.TrimSpace(derefString(sender.SenderType)))
	return senderType == "" || senderType == "user"
}

func extractFeishuSenderID(userID *larkim.UserId) string {
	if userID == nil {
		return ""
	}
	if userID.UserId != nil && *userID.UserId != "" {
		return *userID.UserId
	}
	if userID.OpenId != nil && *userID.OpenId != "" {
		return *userID.OpenId
	}
	if userID.UnionId != nil && *userID.UnionId != "" {
		return *userID.UnionId
	}
	return ""
}

func parseFeishuMessageContent(contentRaw, msgType string) string {
	contentText := contentRaw
	if contentRaw != "" {
		var contentJSON map[string]interface{}
		if err := json.Unmarshal([]byte(contentRaw), &contentJSON); err == nil {
			switch {
			case contentJSON["text"] != nil:
				if text, ok := contentJSON["text"].(string); ok {
					contentText = text
				}
			case contentJSON["image_key"] != nil:
				if imageKey, ok := contentJSON["image_key"].(string); ok {
					contentText = fmt.Sprintf("[Image: %s]", imageKey)
				}
			case contentJSON["file_key"] != nil:
				if fileKey, ok := contentJSON["file_key"].(string); ok {
					contentText = fmt.Sprintf("[File: %s]", fileKey)
				}
			}
		}
	}

	if msgType != "" && msgType != "text" {
		contentText = fmt.Sprintf("[%s] %s", msgType, contentText)
	}

	return contentText
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// Send 发送消息
func (c *FeishuChannel) Send(msg *bus.OutboundMessage) error {
	c.clearTypingIndicator(msg.ChatID)

	contentMap := map[string]string{"text": msg.Content}
	contentBytes, err := json.Marshal(contentMap)
	if err != nil {
		return fmt.Errorf("failed to marshal content: %w", err)
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(msg.ChatID).
			MsgType(larkim.MsgTypeText).
			Content(string(contentBytes)).
			Build()).
		Build()

	resp, err := c.client.Im.Message.Create(context.Background(), req)
	if err != nil {
		return err
	}

	if !resp.Success() {
		return fmt.Errorf("feishu api error: %d %s", resp.Code, resp.Msg)
	}

	return nil
}

func (c *FeishuChannel) addTypingIndicator(messageID, chatID string) {
	if messageID == "" || chatID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(larkim.NewEmojiBuilder().EmojiType(feishuTypingEmoji).Build()).
			Build()).
		Build()

	resp, err := c.client.Im.MessageReaction.Create(ctx, req)
	if err != nil {
		logger.Debug("Failed to add Feishu typing reaction", zap.Error(err))
		return
	}
	if !resp.Success() {
		logger.Debug("Failed to add Feishu typing reaction",
			zap.Int("code", resp.Code),
			zap.String("msg", resp.Msg))
		return
	}

	reactionID := ""
	if resp.Data != nil {
		reactionID = derefString(resp.Data.ReactionId)
	}
	if reactionID == "" {
		return
	}

	c.mu.Lock()
	queue := c.pendingTyping[chatID]
	now := time.Now()
	filtered := make([]feishuTypingReaction, 0, len(queue)+1)
	for _, item := range queue {
		if now.Sub(item.createdAt) <= feishuTypingTTL {
			filtered = append(filtered, item)
		}
	}
	filtered = append(filtered, feishuTypingReaction{
		messageID:  messageID,
		reactionID: reactionID,
		createdAt:  now,
	})
	c.pendingTyping[chatID] = filtered
	c.mu.Unlock()
}

func (c *FeishuChannel) clearTypingIndicator(chatID string) {
	if chatID == "" {
		return
	}

	c.mu.Lock()
	queue := c.pendingTyping[chatID]
	if len(queue) == 0 {
		c.mu.Unlock()
		return
	}

	state := queue[0]
	remaining := queue[1:]
	if len(remaining) == 0 {
		delete(c.pendingTyping, chatID)
	} else {
		c.pendingTyping[chatID] = remaining
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := larkim.NewDeleteMessageReactionReqBuilder().
		MessageId(state.messageID).
		ReactionId(state.reactionID).
		Build()

	resp, err := c.client.Im.MessageReaction.Delete(ctx, req)
	if err != nil {
		logger.Debug("Failed to remove Feishu typing reaction", zap.Error(err))
		return
	}
	if !resp.Success() {
		logger.Debug("Failed to remove Feishu typing reaction",
			zap.Int("code", resp.Code),
			zap.String("msg", resp.Msg))
	}
}

// Stop 停止飞书通道
func (c *FeishuChannel) Stop() error {
	c.mu.Lock()
	wsCancel := c.wsCancel
	c.wsCancel = nil
	server := c.webhookServer
	c.webhookServer = nil
	c.mu.Unlock()

	if wsCancel != nil {
		wsCancel()
	}

	if server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
			logger.Warn("Failed to shutdown Feishu webhook server", zap.Error(err))
		}
	}

	return c.BaseChannelImpl.Stop()
}
