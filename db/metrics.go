package db

import (
	"github.com/prometheus/client_golang/prometheus"
	"harmonylite/telemetry"
)

var (
	conflictsResolvedCounter prometheus.CounterVec

	replicatedWritesTotal   *prometheus.CounterVec
	applyErrorsTotal        *prometheus.CounterVec
	applyDurationSeconds    *prometheus.HistogramVec
)

const (
	strategyLastWriteWins = "last_write_wins"
	OperationUpsert       = "upsert"
	OperationDelete       = "delete"
)

func init() {
	conflictsResolvedCounter = *telemetry.NewCounterVec(
		"harmonylite_conflicts_resolved_total",
		"Number of times an incoming replicated event overwrote an existing local row.",
		[]string{"strategy"},
	)
	conflictsResolvedCounter.WithLabelValues(strategyLastWriteWins).Add(0)

	replicatedWritesTotal = telemetry.NewCounterVec(
		"harmonylite_db_replicated_writes_total",
		"Total number of successful replicated write operations (upserts or deletes) applied to the local database.",
		[]string{"table_name", "operation"},
	)

	applyErrorsTotal = telemetry.NewCounterVec(
		"harmonylite_db_apply_errors_total",
		"Total number of errors encountered while trying to apply replicated changes.",
		[]string{"table_name", "operation"},
	)

	applyDurationSeconds = telemetry.NewHistogramVec(
		"harmonylite_db_apply_duration_seconds",
		"Latency of applying replicated write operations to the local database.",
		[]string{"table_name", "operation"},
		// Buckets for histogram, e.g., .005s, .01s, .025s, .05s, .1s, .25s, .5s, 1s, 2.5s, 5s, 10s
		// These are just example buckets, adjust as needed for expected latencies.
		prometheus.DefBuckets,
	)

	prometheus.MustRegister(conflictsResolvedCounter)
	prometheus.MustRegister(replicatedWritesTotal)
	prometheus.MustRegister(applyErrorsTotal)
	prometheus.MustRegister(applyDurationSeconds)
}

// IncConflictsResolved increments the conflicts_resolved_total counter.
func IncConflictsResolved(strategy string) {
	conflictsResolvedCounter.WithLabelValues(strategy).Inc()
}

// IncReplicatedWrites increments the replicated_writes_total counter.
func IncReplicatedWrites(tableName, operation string) {
	replicatedWritesTotal.WithLabelValues(tableName, operation).Inc()
}

// IncApplyErrors increments the apply_errors_total counter.
func IncApplyErrors(tableName, operation string) {
	applyErrorsTotal.WithLabelValues(tableName, operation).Inc()
}

// ObserveApplyDuration observes the duration for applying a replicated change.
func ObserveApplyDuration(tableName, operation string, durationSeconds float64) {
	applyDurationSeconds.WithLabelValues(tableName, operation).Observe(durationSeconds)
}
