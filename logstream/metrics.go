package logstream

import (
	"github.com/prometheus/client_golang/prometheus"
	"harmonylite/telemetry"
)

var (
	replicationSubscriberLagMessages prometheus.GaugeVec
	replicationSubscriberLagSeconds  prometheus.GaugeVec
	replicationLastProcessedSequence prometheus.GaugeVec
	replicationLastProcessedTimestampSeconds prometheus.GaugeVec

	replicationPublisherStreamTotalMessages    prometheus.GaugeVec
	replicationPublisherLastPublishedSequence  prometheus.GaugeVec
	replicationPublisherLastPublishedTimestampSeconds prometheus.GaugeVec

	// New NATS specific metrics for logstream
	natsStreamInfoMessages          *prometheus.GaugeVec
	natsStreamInfoBytes             *prometheus.GaugeVec
	natsSubscriptionPendingMessages *prometheus.GaugeVec
	natsMessagesPublishedTotal      *prometheus.CounterVec
	natsMessagesReceivedTotal       *prometheus.CounterVec
	logstreamNatsOperationErrorsTotal *prometheus.CounterVec
)

const (
	LogstreamNatsErrorTypePublish   = "publish"
	LogstreamNatsErrorTypeSubscribe = "subscribe"
)

func init() {
	replicationSubscriberLagMessages = *telemetry.NewGaugeVec(
		"harmonylite_replication_subscriber_lag_messages",
		"Number of messages this node is behind on a subscribed NATS stream.",
		[]string{"source_stream_name"},
	)
	replicationSubscriberLagSeconds = *telemetry.NewGaugeVec(
		"harmonylite_replication_subscriber_lag_seconds",
		"Estimated time this node is behind on a subscribed NATS stream.",
		[]string{"source_stream_name"},
	)
	replicationLastProcessedSequence = *telemetry.NewGaugeVec(
		"harmonylite_replication_last_processed_sequence",
		"The last sequence number processed by this node for a given stream.",
		[]string{"source_stream_name"},
	)
	replicationLastProcessedTimestampSeconds = *telemetry.NewGaugeVec(
		"harmonylite_replication_last_processed_timestamp_seconds",
		"Timestamp of the last message successfully processed from a stream.",
		[]string{"source_stream_name"},
	)
	replicationPublisherStreamTotalMessages = *telemetry.NewGaugeVec(
		"harmonylite_replication_publisher_stream_total_messages",
		"Total messages currently in a NATS stream published by this node.",
		[]string{"stream_name"},
	)
	replicationPublisherLastPublishedSequence = *telemetry.NewGaugeVec(
		"harmonylite_replication_publisher_last_published_sequence",
		"Sequence number of the last message successfully published by this node to a stream.",
		[]string{"stream_name"},
	)
	replicationPublisherLastPublishedTimestampSeconds = *telemetry.NewGaugeVec(
		"harmonylite_replication_publisher_last_published_timestamp_seconds",
		"Timestamp of the last message successfully published.",
		[]string{"stream_name"},
	)

	prometheus.MustRegister(replicationSubscriberLagMessages)
	prometheus.MustRegister(replicationSubscriberLagSeconds)
	prometheus.MustRegister(replicationLastProcessedSequence)
	prometheus.MustRegister(replicationLastProcessedTimestampSeconds)
	prometheus.MustRegister(replicationPublisherStreamTotalMessages)
	prometheus.MustRegister(replicationPublisherLastPublishedSequence)
	prometheus.MustRegister(replicationPublisherLastPublishedTimestampSeconds)

	// Initialize new NATS specific metrics for logstream
	natsStreamInfoMessages = telemetry.NewGaugeVec(
		"harmonylite_nats_stream_info_messages",
		"Total messages in the JetStream stream.",
		[]string{"stream_name"},
	)
	natsStreamInfoBytes = telemetry.NewGaugeVec(
		"harmonylite_nats_stream_info_bytes",
		"Total bytes in the JetStream stream.",
		[]string{"stream_name"},
	)
	natsSubscriptionPendingMessages = telemetry.NewGaugeVec(
		"harmonylite_nats_subscription_pending_messages",
		"Number of pending messages for a NATS subscription.",
		[]string{"subject"},
	)
	natsMessagesPublishedTotal = telemetry.NewCounterVec(
		"harmonylite_nats_messages_published_total",
		"Total number of messages successfully published to a NATS stream.",
		[]string{"stream_name"},
	)
	natsMessagesReceivedTotal = telemetry.NewCounterVec(
		"harmonylite_nats_messages_received_total",
		"Total number of messages successfully received and processed from a NATS stream.",
		[]string{"source_stream_name"},
	)
	logstreamNatsOperationErrorsTotal = telemetry.NewCounterVec(
		"harmonylite_logstream_nats_operation_errors_total",
		"Total number of NATS operation errors in logstream.",
		[]string{"type"},
	)

	// Initialize counters for known error types and operations
	logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypePublish).Add(0)
	logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypeSubscribe).Add(0)
	// While not strictly necessary to pre-initialize for labels that will be dynamically added (like stream names),
	// it can be done if there's a known set of streams at startup. For now, we'll let them be created on first use.

	prometheus.MustRegister(natsStreamInfoMessages)
	prometheus.MustRegister(natsStreamInfoBytes)
	prometheus.MustRegister(natsSubscriptionPendingMessages)
	prometheus.MustRegister(natsMessagesPublishedTotal)
	prometheus.MustRegister(natsMessagesReceivedTotal)
	prometheus.MustRegister(logstreamNatsOperationErrorsTotal)
}
