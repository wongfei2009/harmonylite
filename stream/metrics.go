package stream

import (
	"github.com/prometheus/client_golang/prometheus"
	"harmonylite/telemetry"
)

var (
	natsConnectionStatus prometheus.Gauge
	natsErrorsTotal      *prometheus.CounterVec
)

const (
	NatsErrorTypeConnect = "connect"
)

func init() {
	natsConnectionStatus = telemetry.NewGauge(
		"harmonylite_nats_connection_status",
		"NATS connection status (1 for connected, 0 for disconnected).",
	)
	// Initialize to 0 (disconnected) by default
	natsConnectionStatus.Set(0)

	natsErrorsTotal = telemetry.NewCounterVec(
		"harmonylite_nats_errors_total",
		"Total number of NATS related errors.",
		[]string{"type"},
	)
	// Initialize the counter for "connect" type to ensure it's exported.
	natsErrorsTotal.WithLabelValues(NatsErrorTypeConnect).Add(0)

	prometheus.MustRegister(natsConnectionStatus)
	prometheus.MustRegister(natsErrorsTotal)
}

// SetNatsConnectionStatus sets the NATS connection status metric.
func SetNatsConnectionStatus(isConnected bool) {
	if isConnected {
		natsConnectionStatus.Set(1)
	} else {
		natsConnectionStatus.Set(0)
	}
}

// IncNatsErrorsTotal increments the NATS errors total counter for a given type.
func IncNatsErrorsTotal(errorType string) {
	natsErrorsTotal.WithLabelValues(errorType).Inc()
}
