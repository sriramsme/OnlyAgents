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

	"golang.org/x/time/rate"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/api"
	"github.com/sriramsme/OnlyAgents/internal/api/handlers"
	"github.com/sriramsme/OnlyAgents/internal/auth"
	_ "github.com/sriramsme/OnlyAgents/internal/bootstrap"
	"github.com/sriramsme/OnlyAgents/internal/config"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	_ "github.com/sriramsme/OnlyAgents/pkg/skills/bootstrap"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the OnlyAgents server",
}

var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the OnlyAgents server",
	RunE:  runServer,
}

var (
	serverHost string
	serverPort int
	logLevel   string
	logFormat  string
)

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.AddCommand(serverStartCmd)

	serverStartCmd.Flags().StringVar(&serverHost, "host", "0.0.0.0", "Server host")
	serverStartCmd.Flags().IntVarP(&serverPort, "port", "p", 8080, "Server port")
	serverStartCmd.Flags().StringVar(&logLevel, "log-level", "debug", "Log level (debug, info, warn, error)")
	serverStartCmd.Flags().StringVar(&logFormat, "log-format", "json", "Log format (json, text)")
}

func runServer(cmd *cobra.Command, args []string) error {
	logger.Initialize(logLevel, logFormat)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigChan
		logger.Log.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()

	fmt.Println("OnlyAgents Server v0.1.0")
	fmt.Println("=========================")

	uiBus := make(core.UIBus, core.UIBusBuffer)

	k, err := kernel.NewKernel(ctx, cancel, uiBus)
	if err != nil {
		return fmt.Errorf("initialising kernel: %w", err)
	}
	if err := k.Start(); err != nil {
		return fmt.Errorf("starting kernel: %w", err)
	}

	serverConfig, err := loadServerConfig()
	if err != nil {
		return fmt.Errorf("loading server config: %w", err)
	}
	if cmd.Flags().Changed("host") {
		serverConfig.Host = serverHost
	}
	if cmd.Flags().Changed("port") {
		serverConfig.Port = serverPort
	}

	// Auth — initAuth lives in shared.go, also used by cmd/auth.go
	a := initAuth(dataDir())
	defer a.Stop()
	username, err := auth.GetUsername(dataDir())
	if err != nil {
		return fmt.Errorf("loading auth: %w", err)
	}
	fmt.Printf("Username : %s\n", username)
	server := api.NewServer(
		config.ServerConfig{
			Host:         serverConfig.Host,
			Port:         serverConfig.Port,
			APIKeyVault:  "",
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
	fmt.Printf("Server started at %s\n", u.String())
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

	// Stop kernel FIRST — this closes WS connections via OAChannel.Stop()
	if err := k.Stop(); err != nil {
		logger.Log.Error("kernel stop error", "error", err)
	}

	// THEN stop HTTP server — no active WS connections left, exits cleanly
	if err := server.Stop(shutdownCtx); err != nil {
		logger.Log.Error("server stop error", "error", err)
	}

	logger.Log.Info("shutdown complete")
	return nil
}

func loadServerConfig() (*config.ServerConfig, error) {
	return config.LoadServerConfig()
}

// dataDir returns ~/.onlyagents, the canonical data directory.
// All persistent state (auth.yaml, future db, etc.) lives here.
func dataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("cannot determine home directory", "error", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".onlyagents")
}

// initAuth initializes the Auth subsystem.
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
