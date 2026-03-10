package telegram

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// startWebhook starts webhook mode.
//
// Required config fields (under webhook:):
//
//	url:         Full public HTTPS URL Telegram will POST to, e.g. "https://example.com/bot"
//	listen_addr: Local address to bind the HTTP server, e.g. ":8443"
//	path:        URL path component, e.g. "/bot"  (must match the path in `url`)
func (c *TelegramChannel) startWebhook(ctx context.Context) error {
	wh := c.config.Webhook
	if wh == nil {
		return fmt.Errorf("webhook config is required when mode is 'webhook'")
	}
	if wh.URL == "" {
		return fmt.Errorf("webhook.url is required (full public HTTPS URL)")
	}
	if wh.ListenAddr == "" {
		return fmt.Errorf("webhook.listen_addr is required (e.g. ':8443')")
	}
	if wh.Path == "" {
		return fmt.Errorf("webhook.path is required (e.g. '/bot')")
	}

	// Generate a cryptographically secure secret token so we can validate
	// that incoming requests really come from Telegram.
	secretToken := c.bot.SecretToken()

	// Register the webhook URL with Telegram (and optionally drop stale updates).
	if err := c.bot.SetWebhook(ctx, &telego.SetWebhookParams{
		URL:                wh.URL,
		SecretToken:        secretToken,
		DropPendingUpdates: wh.DropPendingUpdates,
		MaxConnections:     wh.MaxConnections,
	}); err != nil {
		return fmt.Errorf("failed to register webhook with Telegram: %w", err)
	}

	c.logger.Info("webhook registered with Telegram",
		"url", wh.URL,
		"drop_pending", wh.DropPendingUpdates,
	)

	// Optionally verify Telegram accepted it.
	info, err := c.bot.GetWebhookInfo(ctx)
	if err != nil {
		c.logger.Warn("could not verify webhook info", "err", err)
	} else if info.LastErrorMessage != "" {
		c.logger.Warn("telegram reports a webhook error",
			"message", info.LastErrorMessage,
			"date", info.LastErrorDate,
		)
	}

	// Set up the local HTTP server that Telegram will call.
	mux := http.NewServeMux()

	updates, err := c.bot.UpdatesViaWebhook(ctx,
		telego.WebhookHTTPServeMux(mux, "POST "+wh.Path, secretToken),
		telego.WithWebhookBuffer(128),
	)
	if err != nil {
		return fmt.Errorf("failed to configure webhook update receiver: %w", err)
	}

	// Build the bot handler from the update channel — identical to polling.
	handler, err := th.NewBotHandler(c.bot, updates)
	if err != nil {
		return fmt.Errorf("failed to create bot handler: %w", err)
	}

	c.handler = handler
	c.registerHandlers()

	srv := &http.Server{
		Addr:              wh.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,  // slowloris protection
		ReadTimeout:       10 * time.Second, // full request read
		WriteTimeout:      15 * time.Second, // response write
		IdleTimeout:       60 * time.Second, // keep-alive
		MaxHeaderBytes:    1 << 20,          // 1MB	}
	}

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	c.logger.Info("telegram bot started",
		"mode", "webhook",
		"username", c.bot.Username(),
		"listen", wh.ListenAddr,
		"path", wh.Path,
	)

	// Run the HTTP server in the background.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Error("webhook HTTP server exited", "err", err)
		}
	}()

	// Run the bot handler in the background.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := handler.Start(); err != nil {
			c.logger.Error("telegram handler exited", "err", err)
		}
	}()

	// Graceful shutdown when the context is cancelled.
	c.wg.Add(1)
	go func(ctx context.Context) {
		defer c.wg.Done()

		<-ctx.Done()
		c.logger.Debug("context cancelled, shutting down webhook server")

		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			c.logger.Error("webhook server shutdown error", "err", err)
		}
		if err := handler.Stop(); err != nil {
			c.logger.Error("telegram handler stop failed", "err", err)
		}
	}(ctx)

	return nil
}
