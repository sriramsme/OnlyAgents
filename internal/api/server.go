package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/sriramsme/OnlyAgents/internal/api/handlers"
	"github.com/sriramsme/OnlyAgents/pkg/config"
)

// Server is the OnlyAgents HTTP server.
type Server struct {
	httpServer *http.Server
	config     config.ServerConfig
	logger     *slog.Logger
}

// NewServer creates a new API server.
// deps holds all the dependencies handlers need.
func NewServer(cfg config.ServerConfig, deps handlers.Deps, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	mid := NewMiddleware(cfg, logger)

	// Register all routes - see routes.go
	registerRoutes(mux, mid, deps, logger)

	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:      mux,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		config: cfg,
		logger: logger,
	}
}

// Start starts the HTTP server (blocking)
func (s *Server) Start() error {
	s.logger.Info("api server listening",
		"addr", s.httpServer.Addr,
		"version", s.config.Version)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("stopping api server")
	return s.httpServer.Shutdown(ctx)
}
