package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

func (c *TelegramChannel) registerHandlers() {
	// Command: /start
	c.handler.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleStart(ctx, &message)
	}, th.CommandEqual("start"))

	// Command: /help
	c.handler.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleHelp(ctx, &message)
	}, th.CommandEqual("help"))

	// All other messages
	c.handler.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleMessage(ctx, &message)
	}, th.AnyMessage())
}

// handleStart handles /start command
func (c *TelegramChannel) handleStart(ctx *th.Context, message *telego.Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	response := "Hello! I'm your OnlyAgents assistant. Send me a message and I'll help you!"

	_, err := c.bot.SendMessage(ctx, tu.Message(
		tu.ID(message.Chat.ID),
		response,
	).WithReplyParameters(&telego.ReplyParameters{
		MessageID: message.MessageID,
	}))

	if err != nil {
		c.logger.Error("failed to send start message", "error", err)
	}

	return nil
}

// handleHelp handles /help command
func (c *TelegramChannel) handleHelp(ctx *th.Context, message *telego.Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	helpText := `Available commands:
/start - Start the bot
/help - Show this help message

Just send me a message and I'll help you with your request!`

	_, err := c.bot.SendMessage(ctx, tu.Message(
		tu.ID(message.Chat.ID),
		helpText,
	).WithReplyParameters(&telego.ReplyParameters{
		MessageID: message.MessageID,
	}))

	if err != nil {
		c.logger.Error("failed to send help message", "error", err)
	}

	return nil
}

// handleMessage handles incoming messages
func (c *TelegramChannel) handleMessage(ctx *th.Context, message *telego.Message) error {
	if message == nil || message.From == nil {
		return fmt.Errorf("invalid message")
	}

	// Check whitelist
	if !c.isAllowed(message.From) {
		c.logger.Debug("message rejected by allowlist",
			"user_id", message.From.ID,
			"username", message.From.Username)
		return nil
	}

	chatID := fmt.Sprintf("%d", message.Chat.ID)
	userID := fmt.Sprintf("%d", message.From.ID)

	// Extract message content
	content := c.extractContent(message)
	if content == "" {
		content = "[empty message]"
	}

	c.logger.Debug("received message",
		"user_id", userID,
		"chat_id", chatID,
		"preview", truncate(content, 50))

	// Route to appropriate agent
	agentID := c.routeMessage(userID, content)
	agent, err := c.agentRegistry.Get(agentID)
	if err != nil {
		c.logger.Error("failed to get agent",
			"agent_id", agentID,
			"available_agents", c.agentRegistry.ListAll(),
			"error", err)
		c.sendErrorResponse(ctx, chatID, message.Chat.ID, "Sorry, I couldn't find an agent to handle your request.")
		return nil
	}

	c.logger.Debug("routing message to agent",
		"agent_id", agentID,
		"user_id", userID)

	// Send "thinking" indicator
	c.sendThinkingIndicator(ctx, chatID, message.Chat.ID)

	// Create placeholder message
	c.createPlaceholder(ctx, chatID, message.Chat.ID)

	// Create agent execution context with timeout from connector context
	// Agent execution may take longer, so use a generous timeout
	agentCtx, agentCancel := context.WithTimeout(c.ctx, 5*time.Minute)
	defer func() { agentCancel() }()

	// Process message through agent
	response, err := agent.Execute(agentCtx, content)

	// Stop thinking indicator
	c.stopThinkingIndicator(chatID)

	if err != nil {
		c.logger.Error("agent execution failed",
			"error", err,
			"agent_id", agentID,
			"user_id", userID)
		response = "Sorry, I encountered an error processing your request."
	}

	// Send response
	if err := c.sendResponse(ctx, chatID, message.Chat.ID, response); err != nil {
		c.logger.Error("failed to send response", "error", err)
	}

	return nil
}

// routeMessage determines which agent should handle the message
func (c *TelegramChannel) routeMessage(userID, content string) string {
	// Executive-driven routing: always route to default agent (usually executive)
	// Executive agent handles delegation to specialized sub-agents

	if c.config.DefaultAgent != "" {
		c.logger.Debug("routing to default agent",
			"agent_id", c.config.DefaultAgent)
		return c.config.DefaultAgent
	}

	// Fallback: use first available agent
	agents := c.agentRegistry.ListAll()
	if len(agents) > 0 {
		c.logger.Debug("routing to first available agent",
			"agent_id", agents[0])
		return agents[0]
	}

	c.logger.Warn("no agents available for routing")
	return "default"
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
		content.WriteString(fmt.Sprintf("[document: %s]", message.Document.FileName))
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

// sendResponse sends the agent's response
func (c *TelegramChannel) sendResponse(ctx *th.Context, chatIDStr string, chatID int64, content string) error {
	// Convert markdown to Telegram HTML
	htmlContent := markdownToTelegramHTML(content)

	// Try to edit placeholder
	if msgID, ok := c.placeholders.LoadAndDelete(chatIDStr); ok {
		editMsg := tu.EditMessageText(
			tu.ID(chatID),
			msgID.(int),
			htmlContent,
		).WithParseMode(telego.ModeHTML)

		_, err := c.bot.EditMessageText(ctx, editMsg)
		if err == nil {
			return nil
		}
		// Fallback to new message if edit fails
		c.logger.Debug("failed to edit placeholder, sending new message", "error", err)
	}

	// Send new message
	msg := tu.Message(tu.ID(chatID), htmlContent).
		WithParseMode(telego.ModeHTML)

	_, err := c.bot.SendMessage(ctx, msg)
	if err != nil {
		// Fallback to plain text if HTML parsing fails
		c.logger.Debug("HTML parse failed, falling back to plain text", "error", err)
		msg.ParseMode = ""
		_, err = c.bot.SendMessage(ctx, msg)
	}

	return err
}

// sendErrorResponse sends an error message (without placeholder editing)
func (c *TelegramChannel) sendErrorResponse(ctx *th.Context, chatIDStr string, chatID int64, errorMsg string) {
	// Clean up placeholder
	c.placeholders.Delete(chatIDStr)
	c.stopThinkingIndicator(chatIDStr)

	msg := tu.Message(tu.ID(chatID), errorMsg)
	if _, err := c.bot.SendMessage(ctx, msg); err != nil {
		c.logger.Error("failed to send error response", "error", err)
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
