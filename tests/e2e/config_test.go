package main_test

import "time"

const (
	// Timeouts and Delays
	maxWaitTime         = 30 * time.Second
	pollInterval        = 500 * time.Millisecond
	nodeStartupDelay    = 10 * time.Second
	nodeShutdownTimeout = 5 * time.Second

	// NATS Configuration
	natsConnectTimeout = 10 * time.Second
	natsRetryAttempts  = 5
	natsRetryDelay     = 2 * time.Second

	// Database Configuration
	dbDir = "/tmp"
)
