package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

func (c *TelegramChannel) registerHandlers() {
	c.handler.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleStart(ctx, &message)
	}, th.CommandEqual("start"))

	c.handler.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleHelp(ctx, &message)
	}, th.CommandEqual("help"))

	c.handler.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleNewSession(ctx, &message)
	}, th.CommandEqual("new-session"))

	c.handler.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleMessage(ctx, &message)
	}, th.AnyMessage())
}

func (c *TelegramChannel) handleStart(ctx *th.Context, message *telego.Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}
	_, err := c.bot.SendMessage(ctx, tu.Message(
		tu.ID(message.Chat.ID),
		"Hello! I'm your OnlyAgents assistant. Send me a message and I'll help you!",
	).WithReplyParameters(&telego.ReplyParameters{MessageID: message.MessageID}))
	return err
}

func (c *TelegramChannel) handleHelp(ctx *th.Context, message *telego.Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}
	helpText := "Available commands:\n/start - Start the bot\n/help - Show this help\n\nJust send me a message!"
	_, err := c.bot.SendMessage(ctx, tu.Message(
		tu.ID(message.Chat.ID),
		helpText,
	).WithReplyParameters(&telego.ReplyParameters{MessageID: message.MessageID}))
	return err
}

// handleMessage is the async entry point for all user messages.
// It fires a MessageReceived event onto the bus and returns immediately.
// The response arrives later via channel.Send() called by kernel.
func (c *TelegramChannel) handleMessage(ctx *th.Context, message *telego.Message) error {
	if message == nil || message.From == nil {
		return fmt.Errorf("invalid message")
	}

	if !c.isAllowed(message.From) {
		c.logger.Debug("message rejected by allowlist",
			"user_id", message.From.ID,
			"username", message.From.Username)
		return nil
	}

	content := c.extractContent(message)
	if content == "" {
		content = "[empty message]"
	}

	chatID := fmt.Sprintf("%d", message.Chat.ID)
	userID := fmt.Sprintf("%d", message.From.ID)

	c.logger.Debug("received message",
		"user_id", userID,
		"chat_id", chatID,
		"preview", truncate(content, 50))

	// Download any attached files before firing the event.
	// By the time the event reaches the kernel, all attachments are on disk.
	attachments, err := c.extractAttachments(ctx.Context(), message)
	if err != nil {
		// extractAttachments only returns a hard error on unexpected conditions;
		// individual file failures are already logged and skipped inside it.
		c.logger.Warn("attachment extraction failed", "err", err)
		// Continue with whatever attachments we managed to save (may be nil).
	}

	if len(attachments) > 0 {
		c.logger.Debug("attachments resolved",
			"count", len(attachments),
			"chat_id", chatID)
	}

	replyToPlatformMessageID := ""
	if message.ReplyToMessage != nil {
		replyToPlatformMessageID = fmt.Sprintf("%d", message.ReplyToMessage.MessageID)
	}

	// Show thinking indicator and create placeholder before firing event.
	// Send() will update this placeholder when the response arrives.
	c.sendThinkingIndicator(ctx, chatID, message.Chat.ID)
	c.createPlaceholder(ctx, chatID, message.Chat.ID)
	c.stopThinkingIndicator(chatID)

	err = c.resolveSessionID(chatID)
	if err != nil {
		return fmt.Errorf("failed to resolve session ID: %w", err)
	}

	// Fire event — kernel routes to agent, agent replies via OutboundMessage → Send()
	c.eventBus <- core.Event{
		Type:          core.MessageReceived,
		CorrelationID: uuid.NewString(),
		Payload: core.MessageReceivedPayload{
			Channel: &core.ChannelMetadata{
				SessionID: c.currentSessionID,
				ChatID:    chatID,
				Name:      "telegram",
				UserID:    userID,
				Username:  message.From.Username,
			},
			Content:                  content,
			Attachments:              attachments,
			ReplyToPlatformMessageID: replyToPlatformMessageID,
			PlatformMessageID:        fmt.Sprintf("%d", message.MessageID),
		},
	}

	return nil
}

func (c *TelegramChannel) handleNewSession(ctx *th.Context, message *telego.Message) error {
	if message == nil || message.From == nil {
		return fmt.Errorf("invalid message")
	}

	agentID := c.config.DefaultAgent
	chatID := fmt.Sprintf("%d", message.Chat.ID)

	replyCh := make(chan core.Event, 1)
	c.eventBus <- core.Event{
		Type:          core.SessionNew,
		CorrelationID: uuid.NewString(),
		ReplyTo:       replyCh,
		Payload: core.SessionNewPayload{
			Channel: "telegram",
			AgentID: agentID,
			ChatID:  chatID,
		},
	}

	select {
	case reply := <-replyCh:
		sessionID, _ := reply.Payload.(string)
		if sessionID == "" {
			_, err := c.bot.SendMessage(ctx, tu.Message(
				tu.ID(message.Chat.ID),
				"❌ Failed to start new session. Please try again.",
			))
			return fmt.Errorf("empty session id from kernel %w", err)
		}
		c.currentSessionID = sessionID
		_, err := c.bot.SendMessage(ctx, tu.Message(
			tu.ID(message.Chat.ID),
			"🆕 New session started. Previous conversation has been archived.",
		))
		return err
	case <-c.ctx.Done():
		return fmt.Errorf("context cancelled")
	}
}

// Send is called by kernel when the agent has a response ready.
// It updates the placeholder message created in handleMessage.

func (c *TelegramChannel) Send(ctx context.Context, msg channels.OutgoingMessage) (channels.SendResult, error) {
	if strings.TrimSpace(msg.Content) == "" {
		c.logger.Warn("empty message, skipping send")
		return channels.SendResult{}, nil
	}
	chatID, err := parseChatID(msg.Channel.ChatID)
	if err != nil {
		return channels.SendResult{}, fmt.Errorf("invalid chat id: %w", err)
	}
	htmlContent := markdownToTelegramHTML(msg.Content)
	htmlContent += agentHeader(msg.AgentID, msg.AgentName)
	for _, att := range msg.Attachments {
		if err := c.sendAttachment(ctx, chatID, att); err != nil {
			c.logger.Warn("failed to send attachment", "attachment_id", att.ID, "err", err)
		}
	}
	switch {
	case len(htmlContent) <= 4096:
		return c.sendSingle(ctx, chatID, msg.Channel.ChatID, htmlContent, msg.AgentID)
	case len(msg.Content) > telegramFileThreshold:
		return c.sendAsFile(ctx, chatID, msg.Channel.ChatID, msg.Content)
	default:
		return c.sendChunked(ctx, chatID, msg.Channel.ChatID, msg.Content, msg.AgentID)
	}
}

func agentHeader(id, name string) string {
	if id == "executive" || name == "" {
		return ""
	}
	return fmt.Sprintf("\n\n<b>— %s</b>", name)
}

// SendToken implements channels.TokenStreamer.
// Called by kernel for each streaming token — edits the placeholder in place.
// In TelegramChannel struct

func (c *TelegramChannel) SendToken(ctx context.Context, chatID, token, accumulated string) error {
	// Cancel pending edit and schedule a new one 300ms out
	if t, ok := c.tokenDebounce.Load(chatID); ok {
		t.(*time.Timer).Stop()
	}
	timer := time.AfterFunc(300*time.Millisecond, func() {
		c.tokenDebounce.Delete(chatID)
		c.editPlaceholder(ctx, chatID, accumulated)
	})
	c.tokenDebounce.Store(chatID, timer)
	return nil
}

func (c *TelegramChannel) editPlaceholder(ctx context.Context, chatID, content string) {
	msgID, ok := c.placeholders.Load(chatID)
	if !ok {
		c.logger.Error("telegram: editPlaceholder: placeholder not found", "chat_id", chatID)
		return
	}
	id, err := parseChatID(chatID)
	if err != nil {
		c.logger.Error("telegram: editPlaceholder: parseChatID failed", "err", err)
		return
	}
	htmlContent := markdownToTelegramHTML(content)
	_, err = c.bot.EditMessageText(ctx, tu.EditMessageText(
		tu.ID(id), msgID.(int), htmlContent,
	).WithParseMode(telego.ModeHTML))
	if err != nil {
		c.logger.Error("telegram: editPlaceholder: EditMessageText failed", "err", err)
		return
	}
}

// extractContent extracts text content from a message
func (c *TelegramChannel) extractContent(message *telego.Message) string {
	var content strings.Builder

	if message.Text != "" {
		content.WriteString(message.Text)
	}

	if message.Caption != "" {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString(message.Caption)
	}

	// TODO: Handle photos, voice, documents, etc.
	if len(message.Photo) > 0 {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString("[image attached]")
	}

	if message.Voice != nil {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		content.WriteString("[voice message]")
	}

	if message.Document != nil {
		if content.Len() > 0 {
			content.WriteString("\n")
		}
		fmt.Fprintf(&content, "[document: %s]", message.Document.FileName)
	}

	return content.String()
}

// sendThinkingIndicator sends typing action
func (c *TelegramChannel) sendThinkingIndicator(ctx *th.Context, chatIDStr string, chatID int64) {
	// Create a context for the thinking indicator lifecycle
	thinkingCtx, cancel := context.WithTimeout(c.ctx, 5*time.Minute)
	c.thinkingCtx.Store(chatIDStr, cancel)

	// Send initial typing action
	if err := c.bot.SendChatAction(ctx, tu.ChatAction(tu.ID(chatID), telego.ChatActionTyping)); err != nil {
		c.logger.Debug("typing action failed", "err", err)
	}

	// Continue sending typing action every 4 seconds
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Create short-lived context for each API call
				tickCtx, tickCancel := context.WithTimeout(thinkingCtx, 5*time.Second)
				if err := c.bot.SendChatAction(tickCtx, tu.ChatAction(tu.ID(chatID), telego.ChatActionTyping)); err != nil {
					c.logger.Debug("typing action failed", "err", err)
				}
				tickCancel()
			case <-thinkingCtx.Done():
				return
			}
		}
	}()
}

// stopThinkingIndicator stops the typing animation
func (c *TelegramChannel) stopThinkingIndicator(chatIDStr string) {
	if cancel, ok := c.thinkingCtx.LoadAndDelete(chatIDStr); ok {
		if cancelFunc, ok := cancel.(context.CancelFunc); ok {
			cancelFunc()
		}
	}
}

// createPlaceholder creates a "thinking" placeholder message
func (c *TelegramChannel) createPlaceholder(ctx *th.Context, chatIDStr string, chatID int64) {
	msg, err := c.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), "💭 Thinking..."))
	if err == nil {
		c.placeholders.Store(chatIDStr, msg.MessageID)
	}
}

// isAllowed checks if a user is allowed to use the bot
func (c *TelegramChannel) isAllowed(user *telego.User) bool {
	if len(c.config.AllowFrom) == 0 {
		return true // No whitelist = everyone allowed
	}

	userIDStr := fmt.Sprintf("%d", user.ID)
	username := user.Username

	for _, allowed := range c.config.AllowFrom {
		if allowed == userIDStr || allowed == username {
			return true
		}
	}

	return false
}
