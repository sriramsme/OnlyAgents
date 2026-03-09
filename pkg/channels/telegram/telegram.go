package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

const (
	version = "1.0.0"
)

func init() {
	channels.Register("telegram", NewChannel)
}

// Connector implements the Channel interface for Telegram
type TelegramChannel struct {
	config   *Config // telegram.Config, not connectors.TelegramConfig
	vault    vault.Vault
	eventBus chan<- core.Event
	bot      *telego.Bot
	handler  *th.BotHandler

	tokenDebounce sync.Map // chatID → *time.Timer

	// State
	mu      sync.RWMutex
	running bool

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Logging
	logger *slog.Logger

	// Message tracking
	placeholders sync.Map // chatID -> messageID
	thinkingCtx  sync.Map // chatID -> cancelFunc
}

// NewChannel creates a new Telegram channel
func NewChannel(
	ctx context.Context,
	rawConfig map[string]interface{},
	vault vault.Vault,
	eventBus chan<- core.Event,
) (channels.Channel, error) {

	var cfg Config

	// Decode raw config into telegram-specific Config struct
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &cfg,
		WeaklyTypedInput: true,
		TagName:          "yaml",
	})
	if err != nil {
		return nil, fmt.Errorf("create decoder: %w", err)
	}

	if err := decoder.Decode(rawConfig); err != nil {
		return nil, fmt.Errorf("decode telegram config: %w", err)
	}

	channelCtx, cancel := context.WithCancel(ctx)
	logger := slog.With(
		"connector", "telegram",
	)

	return &TelegramChannel{
		config:       &cfg,
		vault:        vault,
		eventBus:     eventBus,
		ctx:          channelCtx,
		cancel:       cancel,
		logger:       logger,
		placeholders: sync.Map{},
		thinkingCtx:  sync.Map{},
	}, nil
}

// PlatformName returns the platform name
func (c *TelegramChannel) PlatformName() string {
	return "telegram"
}

// Version returns the connector version
func (c *TelegramChannel) Version() string {
	return version
}

// HealthCheck returns true if the connector is healthy
func (c *TelegramChannel) HealthCheck() (bool, error) {
	return true, nil
}

// Telegram adapter — one session per chat, auto-created if not exists
func (c *TelegramChannel) resolveSessionID(agentID string) (string, error) {
	if agentID == "" {
		return "", fmt.Errorf("agentID is empty")
	}
	replyCh := make(chan core.Event, 1)
	c.eventBus <- core.Event{
		Type:    core.SessionGet,
		ReplyTo: replyCh,
		Payload: core.SessionGetPayload{
			Channel: "telegram",
			AgentID: agentID,
		},
	}
	select {
	case reply := <-replyCh:
		sessionID, _ := reply.Payload.(string)
		if sessionID == "" {
			return "", fmt.Errorf("empty session id from kernel")
		}
		return sessionID, nil
	case <-c.ctx.Done():
		return "", fmt.Errorf("context cancelled waiting for session")
	}
}

// Connect initializes the Telegram bot connection
func (c *TelegramChannel) Connect() error {
	c.logger.Info("connecting to telegram")

	// Get bot token from vault (always required)
	if c.config.Credentials.BotToken == "" {
		return fmt.Errorf("bot token vault key is required in credentials")
	}

	botToken, err := c.vault.GetSecret(c.ctx, c.config.Credentials.BotToken)
	if err != nil {
		return fmt.Errorf("failed to get bot token from vault (key: %s): %w",
			c.config.Credentials.BotToken, err)
	}

	if botToken == "" {
		return fmt.Errorf("telegram bot token is empty in vault")
	}

	// Configure bot options
	var opts []telego.BotOption

	// Proxy support
	if c.config.Proxy != "" {
		proxyURL, err := url.Parse(c.config.Proxy)
		if err != nil {
			return fmt.Errorf("invalid proxy URL %q: %w", c.config.Proxy, err)
		}
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}))
		c.logger.Info("using proxy", "proxy", c.config.Proxy)
	}

	// Create bot
	bot, err := telego.NewBot(botToken, opts...)
	if err != nil {
		return fmt.Errorf("failed to create telegram bot: %w", err)
	}

	c.bot = bot
	c.logger.Info("telegram bot created", "username", bot.Username())

	return nil
}

// Disconnect closes the Telegram bot connection
func (c *TelegramChannel) Disconnect() error {
	c.logger.Info("disconnecting from telegram")

	// Stop any running handlers
	if c.handler != nil {
		if err := c.handler.Stop(); err != nil {
			c.logger.Warn("telegram handler stop failed", "err", err)
		}
		c.handler = nil
	}

	// Cancel all thinking animations
	c.thinkingCtx.Range(func(key, value interface{}) bool {
		if cancel, ok := value.(context.CancelFunc); ok {
			cancel()
		}
		c.thinkingCtx.Delete(key)
		return true
	})

	c.bot = nil
	c.logger.Info("disconnected from telegram")

	return nil
}

// Start starts receiving messages
func (c *TelegramChannel) Start() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("connector already running")
	}
	c.mu.Unlock()

	if c.bot == nil {
		return fmt.Errorf("bot not connected, call Connect() first")
	}

	mode := c.config.Mode
	if mode == "" {
		mode = "polling" // Default to polling
	}

	c.logger.Info("starting telegram connector", "mode", mode)

	switch mode {
	case "polling":
		return c.startPolling(c.ctx)
	case "webhook":
		return c.startWebhook(c.ctx)
	default:
		return fmt.Errorf("unsupported mode: %s (use 'polling' or 'webhook')", mode)
	}
}

// Stop stops the connector
func (c *TelegramChannel) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	c.logger.Info("stopping telegram connector")

	// Stop handler
	if c.handler != nil {
		if err := c.handler.Stop(); err != nil {
			c.logger.Warn("telegram handler stop failed", "err", err)
		}
	}

	// Cancel context
	c.cancel()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("telegram connector stopped successfully")
		return nil
	case <-time.After(5 * time.Second):
		c.logger.Warn("telegram connector stop timeout")
		return fmt.Errorf("shutdown timeout")
	}
}
