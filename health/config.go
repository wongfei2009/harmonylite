package health

import "github.com/wongfei2009/harmonylite/cfg"

// DefaultConfig returns a default configuration for the health check service
func DefaultConfig() *cfg.HealthCheckConfiguration {
	return &cfg.HealthCheckConfiguration{
		Enable:   false,  // Disabled by default
		Bind:     "0.0.0.0:8090",
		Path:     "/health",
		Detailed: true,
	}
}
