package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/cfg"
)

// HealthServer manages the HTTP server for health checks
type HealthServer struct {
	config    *cfg.HealthCheckConfiguration
	checker   *HealthChecker
	server    *http.Server
	startTime time.Time
}

// NewHealthServer creates a new HealthServer instance
func NewHealthServer(config *cfg.HealthCheckConfiguration, checker *HealthChecker) *HealthServer {
	return &HealthServer{
		config:    config,
		checker:   checker,
		startTime: time.Now(),
	}
}

// Start starts the health check server
func (s *HealthServer) Start() error {
	if !s.config.Enable {
		log.Info().Msg("Health check server is disabled")
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handleHealthCheck)

	s.server = &http.Server{
		Addr:    s.config.Bind,
		Handler: mux,
	}

	go func() {
		log.Info().
			Str("bind", s.config.Bind).
			Str("path", s.config.Path).
			Msg("Starting health check server")

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Health check server error")
		}
	}()

	return nil
}

// Stop stops the health check server
func (s *HealthServer) Stop() error {
	if s.server == nil {
		return nil
	}

	log.Info().Msg("Stopping health check server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// handleHealthCheck handles HTTP requests for health checks
func (s *HealthServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	status := s.checker.Check()

	if status.Status != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if s.config.Detailed {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}
