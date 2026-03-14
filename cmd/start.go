package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/api"
	"github.com/sriramsme/OnlyAgents/internal/api/handlers"
	"github.com/sriramsme/OnlyAgents/internal/auth"
	"github.com/sriramsme/OnlyAgents/internal/config"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	_ "github.com/sriramsme/OnlyAgents/pkg/skills/bootstrap"
	"golang.org/x/time/rate"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start OnlyAgents (kernel + server)",
	Long: `Start the OnlyAgents kernel and web server.

Use --no-server to run the kernel only (headless mode, useful when
using Telegram or another channel without the built-in web UI).`,
	RunE: runStart,
}

var (
	startHost             string
	startPort             int
	startLogLevel         string
	startLogFormat        string
	startNoServer         bool
	startLogDetailed      bool
	startLogDetailedLLM   bool
	startLogDetailedTools bool
)

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVar(&startHost, "host", "0.0.0.0", "Server host")
	startCmd.Flags().IntVarP(&startPort, "port", "p", 8080, "Server port")
	startCmd.Flags().StringVar(&startLogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	startCmd.Flags().StringVar(&startLogFormat, "log-format", "text", "Log format (json, text)")
	startCmd.Flags().BoolVar(&startNoServer, "no-server", false, "Run kernel only, no web server (headless mode)")
	startCmd.Flags().BoolVar(&startLogDetailed, "log-detailed", false, "Detailed logging for both LLM and tools")
	startCmd.Flags().BoolVar(&startLogDetailedLLM, "log-detailed-llm", false, "Detailed LLM calls")
	startCmd.Flags().BoolVar(&startLogDetailedTools, "log-detailed-tools", false, "Detailed tool calls")
}

// nolint:gocyclo
func runStart(cmd *cobra.Command, args []string) error {
	logger.Initialize(startLogLevel, startLogFormat)
	if startLogDetailed {
		logger.SetTimingDetail(true, true)
	} else {
		logger.SetTimingDetail(startLogDetailedLLM, startLogDetailedTools)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigChan
		logger.Log.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()

	fmt.Println("OnlyAgents v0.1.0")
	fmt.Println("=================")

	// ── Kernel ────────────────────────────────────────────────────────────────
	var uiBus core.UIBus
	if !startNoServer {
		uiBus = make(core.UIBus, core.UIBusBuffer)
	}

	k, err := kernel.NewKernel(ctx, cancel, uiBus)
	if err != nil {
		return fmt.Errorf("initialising kernel: %w", err)
	}

	// ── Boot kernel ─────────────────────────────────────────────────────────
	if err := k.Boot(); err != nil {
		return fmt.Errorf("booting kernel: %w", err)
	}

	if err := k.Run(); err != nil {
		return fmt.Errorf("starting kernel: %w", err)
	}

	// ── Headless mode ─────────────────────────────────────────────────────────
	if startNoServer {
		fmt.Println("Running in headless mode — press Ctrl+C to stop")
		<-ctx.Done()
		logger.Log.Info("shutting down")
		if err := k.Shutdown(); err != nil {
			logger.Log.Error("kernel stop error", "error", err)
		}
		logger.Log.Info("shutdown complete")
		return nil
	}

	// ── Server ────────────────────────────────────────────────────────────────
	serverConfig, err := loadServerConfig()
	if err != nil {
		return fmt.Errorf("loading server config: %w", err)
	}
	if cmd.Flags().Changed("host") {
		serverConfig.Host = startHost
	}
	if cmd.Flags().Changed("port") {
		serverConfig.Port = startPort
	}

	a := initAuth(dataDir())
	defer a.Stop()

	username, err := auth.GetUsername(dataDir())
	if err != nil {
		return fmt.Errorf("loading auth: %w", err)
	}

	server := api.NewServer(
		config.ServerConfig{
			Host:         serverConfig.Host,
			Port:         serverConfig.Port,
			Version:      "0.1.0",
			ReadTimeout:  serverConfig.ReadTimeout,
			WriteTimeout: serverConfig.WriteTimeout,
			IdleTimeout:  serverConfig.IdleTimeout,
		},
		handlers.Deps{
			Bus:     k.Bus(),
			Version: "0.1.0",
			Kernel:  k,
		},
		a,
		logger.Log,
	)

	u := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(serverConfig.Host, strconv.Itoa(serverConfig.Port)),
	}
	fmt.Printf("Username : %s\n", username)
	fmt.Printf("Web UI   : %s\n", u.String())
	fmt.Println("Press Ctrl+C to stop")

	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Start() }()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Log.Info("shutdown initiated")
	}

	fmt.Println("\nShutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Kernel first — closes WS connections before HTTP server stops
	if err := k.Shutdown(); err != nil {
		logger.Log.Error("kernel stop error", "error", err)
	}
	if err := server.Stop(shutdownCtx); err != nil {
		logger.Log.Error("server stop error", "error", err)
	}

	logger.Log.Info("shutdown complete")
	return nil
}

// ── Shared helpers (used by start.go and auth.go) ─────────────────────────────

func loadServerConfig() (*config.ServerConfig, error) {
	return config.LoadServerConfig()
}

func dataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("cannot determine home directory", "error", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".onlyagents")
}

func initAuth(dir string) *auth.Auth {
	if _, err := os.Stat(filepath.Join(dir, "auth.yaml")); err != nil {
		slog.Error("auth not configured — run `onlyagents setup` first")
		os.Exit(1)
	}
	limiter := auth.NewIPRateLimiter(rate.Every(60*time.Second), 5)
	a := auth.New(dir, limiter)
	a.Start()
	return a
}
