package cmd

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/api"
	"github.com/sriramsme/OnlyAgents/internal/api/handlers"
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
	Long:  `Start the OnlyAgents server with API endpoints and kernel`,
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

	// Create UIBus — this is what makes it "server mode".
	// The kernel starts runUI() only when uiBus is non-nil.
	// cmd/agents/main.go passes nil → headless, zero overhead.
	uiBus := make(core.UIBus, core.UIBusBuffer)

	k, err := kernel.NewKernel(ctx, cancel, uiBus)
	if err != nil {
		logger.Log.Error("failed to initialize kernel", "error", err)
		return err
	}

	if err := k.Start(); err != nil {
		logger.Log.Error("failed to start kernel", "error", err)
		return err
	}

	serverConfig, err := loadServerConfig()
	if err != nil {
		logger.Log.Error("failed to load server config", "error", err)
		return err
	}

	if cmd.Flags().Changed("host") {
		serverConfig.Host = serverHost
	}
	if cmd.Flags().Changed("port") {
		serverConfig.Port = serverPort
	}

	server := createAPIServer(serverConfig, k)

	u := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(serverConfig.Host, strconv.Itoa(serverConfig.Port)),
	}
	fmt.Printf("Server started at %s\n", u.String())
	fmt.Println("Press Ctrl+C to stop")

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	select {
	case err := <-serverErr:
		logger.Log.Error("server error", "error", err)
		return err
	case <-ctx.Done():
		logger.Log.Info("shutdown initiated")
	}

	fmt.Println("\nShutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Stop(shutdownCtx); err != nil {
		logger.Log.Error("server stop error", "error", err)
	}

	logger.Log.Info("shutting down kernel")
	if err := k.Stop(); err != nil {
		logger.Log.Error("error shutting down kernel", "error", err)
		return err
	}

	logger.Log.Info("shutdown complete")
	return nil
}

func loadServerConfig() (*config.ServerConfig, error) {
	return config.LoadServerConfig()
}

func createAPIServer(cfg *config.ServerConfig, k *kernel.Kernel) *api.Server {
	return api.NewServer(
		config.ServerConfig{
			Host:        cfg.Host,
			Port:        cfg.Port,
			APIKeyVault: "", // cfg.APIKeyVault,
			Version:     "0.1.0",
		},
		handlers.Deps{
			Bus:     k.Bus(),
			Version: "0.1.0",
			Kernel:  k, // k implements KernelReader — Agents(), IsHealthy(), Subscribe()
		},
		logger.Log,
	)
}
