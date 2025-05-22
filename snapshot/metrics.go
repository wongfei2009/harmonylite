package snapshot

import (
	"github.com/prometheus/client_golang/prometheus"
	"harmonylite/telemetry"
)

// Operation types
const (
	OperationCreate  = "create"
	OperationRestore = "restore"
)

// Phase types for errors
const (
	PhaseCreateTempDir       = "temp_dir"       // Common for create and restore
	PhaseCreateLocalBackup   = "local_backup"   // Create specific
	PhaseCreateUpload        = "upload"         // Create specific
	PhaseRestoreDownload     = "download"       // Restore specific
	PhaseRestoreLocalRestore = "local_restore"  // Restore specific
)

var (
	snapshotCreationTotal              prometheus.Counter
	snapshotCreationDurationSeconds    prometheus.Histogram
	snapshotRestorationTotal           prometheus.Counter
	snapshotRestorationDurationSeconds prometheus.Histogram
	snapshotErrorsTotal                *prometheus.CounterVec
)

func init() {
	snapshotCreationTotal = telemetry.NewCounter(
		"harmonylite_snapshot_creation_total",
		"Total number of successful snapshot creations.",
	)
	snapshotCreationDurationSeconds = telemetry.NewHistogram(
		"harmonylite_snapshot_creation_duration_seconds",
		"Duration of the entire snapshot creation process (local backup + upload).",
		prometheus.DefBuckets, // Default buckets, can be customized
	)
	snapshotRestorationTotal = telemetry.NewCounter(
		"harmonylite_snapshot_restoration_total",
		"Total number of successful snapshot restorations.",
	)
	snapshotRestorationDurationSeconds = telemetry.NewHistogram(
		"harmonylite_snapshot_restoration_duration_seconds",
		"Duration of the entire snapshot restoration process (download + local restore).",
		prometheus.DefBuckets, // Default buckets, can be customized
	)
	snapshotErrorsTotal = telemetry.NewCounterVec(
		"harmonylite_snapshot_errors_total",
		"Total number of errors encountered during snapshot operations.",
		[]string{"operation", "phase"},
	)

	// Initialize known error types to ensure they are exported
	snapshotErrorsTotal.WithLabelValues(OperationCreate, PhaseCreateTempDir).Add(0)
	snapshotErrorsTotal.WithLabelValues(OperationCreate, PhaseCreateLocalBackup).Add(0)
	snapshotErrorsTotal.WithLabelValues(OperationCreate, PhaseCreateUpload).Add(0)
	snapshotErrorsTotal.WithLabelValues(OperationRestore, PhaseCreateTempDir).Add(0) // temp_dir is a shared phase name
	snapshotErrorsTotal.WithLabelValues(OperationRestore, PhaseRestoreDownload).Add(0)
	snapshotErrorsTotal.WithLabelValues(OperationRestore, PhaseRestoreLocalRestore).Add(0)

	prometheus.MustRegister(snapshotCreationTotal)
	prometheus.MustRegister(snapshotCreationDurationSeconds)
	prometheus.MustRegister(snapshotRestorationTotal)
	prometheus.MustRegister(snapshotRestorationDurationSeconds)
	prometheus.MustRegister(snapshotErrorsTotal)
}

// IncSnapshotCreationTotal increments the snapshot creation counter.
func IncSnapshotCreationTotal() {
	snapshotCreationTotal.Inc()
}

// ObserveSnapshotCreationDuration observes the snapshot creation duration.
func ObserveSnapshotCreationDuration(duration float64) {
	snapshotCreationDurationSeconds.Observe(duration)
}

// IncSnapshotRestorationTotal increments the snapshot restoration counter.
func IncSnapshotRestorationTotal() {
	snapshotRestorationTotal.Inc()
}

// ObserveSnapshotRestorationDuration observes the snapshot restoration duration.
func ObserveSnapshotRestorationDuration(duration float64) {
	snapshotRestorationDurationSeconds.Observe(duration)
}

// IncSnapshotError increments the snapshot error counter for a given operation and phase.
func IncSnapshotError(operation, phase string) {
	snapshotErrorsTotal.WithLabelValues(operation, phase).Inc()
}
