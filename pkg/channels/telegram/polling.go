package telegram

import (
	"context"
	"fmt"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// startPolling starts long polling mode
func (c *TelegramChannel) startPolling(ctx context.Context) error {
	// Ensure webhook is disabled
	err := c.bot.DeleteWebhook(ctx, &telego.DeleteWebhookParams{
		DropPendingUpdates: true,
	})
	if err != nil {
		return fmt.Errorf("failed to delete webhook before polling: %w", err)
	}
	timeout := 30
	if c.config.Polling != nil && c.config.Polling.Timeout > 0 {
		timeout = c.config.Polling.Timeout
	}

	updates, err := c.bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
		Timeout: timeout,
	})
	if err != nil {
		return fmt.Errorf("failed to start long polling: %w", err)
	}

	handler, err := th.NewBotHandler(c.bot, updates)
	if err != nil {
		return fmt.Errorf("failed to create bot handler: %w", err)
	}

	c.handler = handler

	// Register command handlers
	c.registerHandlers()

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	c.logger.Info("telegram bot started", "mode", "polling", "username", c.bot.Username())

	// Start handler in background
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		if err := handler.Start(); err != nil {
			c.logger.Error("telegram handler exited", "err", err)
		}
	}()

	// Monitor context cancellation
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		<-ctx.Done()
		c.logger.Debug("context cancelled, stopping handler")

		if err := handler.Stop(); err != nil {
			c.logger.Error("telegram handler stop failed", "err", err)
		}
	}()

	return nil
}
