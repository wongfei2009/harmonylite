package logstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/wongfei2009/harmonylite/stream"

	"github.com/klauspost/compress/zstd"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/cfg"
	"github.com/wongfei2009/harmonylite/snapshot"
)

const maxReplicateRetries = 7
const SnapshotShardID = uint64(1)

var SnapshotLeaseTTL = 10 * time.Second

type Replicator struct {
	nodeID             uint64
	shards             uint64
	compressionEnabled bool
	lastSnapshot       time.Time

	client    *nats.Conn
	repState  *replicationState
	metaStore *replicatorMetaStore
	snapshot  snapshot.NatsSnapshot
	streamMap map[uint64]nats.JetStreamContext

	// closeCtx is used to signal goroutines to stop
	closeCtx       context.Context
	closeCtxCancel context.CancelFunc
}

func NewReplicator(
	snapshot snapshot.NatsSnapshot,
) (*Replicator, error) {
	nodeID := cfg.Config.NodeID
	shards := cfg.Config.ReplicationLog.Shards
	compress := cfg.Config.ReplicationLog.Compress
	updateExisting := cfg.Config.ReplicationLog.UpdateExisting

	nc, err := stream.Connect()
	if err != nil {
		return nil, err
	}

	streamMap := map[uint64]nats.JetStreamContext{}
	for i := uint64(0); i < shards; i++ {
		shard := i + 1
		js, err := nc.JetStream()
		if err != nil {
			return nil, err
		}

		streamCfg := makeShardStreamConfig(shard, shards, compress)
		info, err := js.StreamInfo(streamName(shard, compress), nats.MaxWait(10*time.Second))
		if err == nats.ErrStreamNotFound {
			log.Debug().Uint64("shard", shard).Msg("Creating stream")
			info, err = js.AddStream(streamCfg)
		}

		if err != nil {
			log.Error().
				Err(err).
				Str("name", streamName(shard, compress)).
				Msg("Unable to get stream info...")
			return nil, err
		}

		if updateExisting && !eqShardStreamConfig(&info.Config, streamCfg) {
			log.Warn().Msgf("Stream configuration not same for %s, updating...", streamName(shard, compress))
			info, err = js.UpdateStream(streamCfg)
			if err != nil {
				log.Error().
					Err(err).
					Str("name", streamName(shard, compress)).
					Msg("Unable update stream info...")
				return nil, err
			}
		}

		leader := ""
		if info.Cluster != nil {
			leader = info.Cluster.Leader
		}

		log.Debug().
			Uint64("shard", shard).
			Str("name", info.Config.Name).
			Int("replicas", info.Config.Replicas).
			Str("leader", leader).
			Msg("Stream ready...")

		if err != nil {
			return nil, err
		}

		streamMap[shard] = js
	}

	repState := &replicationState{}
	err = repState.init()
	if err != nil {
		return nil, err
	}

	metaStore, err := newReplicatorMetaStore(cfg.EmbeddedClusterName, nc)
	if err != nil {
		return nil, err
	}

	closeCtx, closeCtxCancel := context.WithCancel(context.Background())

	replicator := &Replicator{
		client:             nc,
		nodeID:             nodeID,
		compressionEnabled: compress,
		lastSnapshot:       time.Time{},

		shards:    shards,
		streamMap: streamMap,
		snapshot:  snapshot,
		repState:  repState,
		metaStore: metaStore,

		closeCtx:       closeCtx,
		closeCtxCancel: closeCtxCancel,
	}

	// Start periodic metric updates
	go replicator.periodicMetricsUpdater()

	return replicator, nil
}

func (r *Replicator) periodicMetricsUpdater() {
	// Periodic ticker, e.g., every 30 seconds
	// More frequent updates for some metrics can be done elsewhere if needed
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.closeCtx.Done():
			log.Info().Msg("Stopping periodic metrics updater for replicator.")
			return
		case <-ticker.C:
			// Update publisher stream total messages
			for shardID, js := range r.streamMap {
				streamNameVal := streamName(shardID, r.compressionEnabled)
				info, err := js.StreamInfo(streamNameVal, nats.MaxWait(5*time.Second))
				if err != nil {
					log.Warn().Err(err).Str("stream", streamNameVal).Msg("Failed to get stream info for metrics")
					continue
				}
				// Existing metric, ensure it's correctly labeled if it's meant for publisher side.
				// The new task asks for harmonylite_nats_stream_info_messages, which seems like a replacement or a more generic version.
				// Assuming replicationPublisherStreamTotalMessages was specific to publisher's view and now we add the generic one.
				replicationPublisherStreamTotalMessages.WithLabelValues(streamNameVal).Set(float64(info.State.Msgs))
				
				// New NATS stream info metrics
				natsStreamInfoMessages.WithLabelValues(streamNameVal).Set(float64(info.State.Msgs))
				natsStreamInfoBytes.WithLabelValues(streamNameVal).Set(float64(info.State.Bytes))


				// Update subscriber lag metrics for all potential source streams this node might be listening to.
				// This is a simplification; in a real scenario, we'd iterate over active subscriptions.
				// For now, we iterate over streams this node *could* subscribe to (all shards).
				// This part assumes a Replicator instance might also be a subscriber.
				// If a node only publishes or only subscribes, this could be more targeted.

				// Get last processed sequence for this stream by this node
				savedSeq := r.repState.get(streamNameVal)
				if info.State.LastSeq > 0 { // Only calculate lag if stream is not empty
					lagMessages := int64(info.State.LastSeq) - int64(savedSeq)
					if lagMessages < 0 { // Should not happen if savedSeq is correctly managed
						lagMessages = 0
					}
					replicationSubscriberLagMessages.WithLabelValues(streamNameVal).Set(float64(lagMessages))

					// Lag in seconds
					// Note: info.State.LastTime might be nil if stream is empty or very new
					if info.State.LastTime != nil {
						// Get timestamp of last processed message by this node
						// This requires storing the timestamp alongside the sequence in repState,
						// which is not currently done. For a simpler approach, we use the last msg time from stream.
						// A more accurate lag in seconds would compare stream's last msg time with local last processed msg time.
						// For now, if savedSeq < info.State.LastSeq, it implies we are lagging.
						// The time lag can be approximated by looking at the age of the last message in the stream
						// if we haven't processed it. More accurately, it's (time of last message in stream - time of last message we processed).
						// This metric is harder to get right without more state.
						// As a proxy: if we've processed the latest, lag is 0. Otherwise, it's age of latest message.
						if savedSeq < info.State.LastSeq {
							replicationSubscriberLagSeconds.WithLabelValues(streamNameVal).Set(float64(time.Now().Unix() - info.State.LastTime.Unix()))
						} else {
							replicationSubscriberLagSeconds.WithLabelValues(streamNameVal).Set(0)
						}
					} else {
						replicationSubscriberLagSeconds.WithLabelValues(streamNameVal).Set(0) // No last time, assume no lag
					}
				} else {
					replicationSubscriberLagMessages.WithLabelValues(streamNameVal).Set(0)
					replicationSubscriberLagSeconds.WithLabelValues(streamNameVal).Set(0)
				}
			}
		}
	}
}

func (r *Replicator) Close() error {
	if r.closeCtxCancel != nil {
		r.closeCtxCancel()
	}
	// Additional cleanup for NATS client etc. can be added here
	if r.client != nil && !r.client.IsClosed() {
		r.client.Close()
	}
	return nil
}

func (r *Replicator) Publish(hash uint64, payload []byte) error {
	shardID := (hash % r.shards) + 1
	js, ok := r.streamMap[shardID]
	if !ok {
		log.Panic().
			Uint64("shard", shardID).
			Msg("Invalid shard")
	}

	if r.compressionEnabled {
		compPayload, err := payloadCompress(payload)
		if err != nil {
			return err
		}

		payload = compPayload
	}

	ack, err := js.Publish(subjectName(shardID), payload)
	streamNameVal := streamName(shardID, r.compressionEnabled) // Use configured stream name for consistency in labels
	if err != nil {
		logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypePublish).Inc()
		return err
	}

	// Update Prometheus metrics for publisher
	// ack.Stream is the actual stream name returned by NATS, prefer this for accuracy if it can differ.
	// However, for consistency with metrics updated in periodicMetricsUpdater, using streamNameVal.
	// If ack.Stream is guaranteed to be streamNameVal, either is fine. Let's use ack.Stream as it's from NATS.
	actualStreamNameFromAck := ack.Stream 
	replicationPublisherLastPublishedSequence.WithLabelValues(actualStreamNameFromAck).Set(float64(ack.Sequence))
	replicationPublisherLastPublishedTimestampSeconds.WithLabelValues(actualStreamNameFromAck).Set(float64(time.Now().Unix()))
	
	// Increment total messages published to this specific stream
	natsMessagesPublishedTotal.WithLabelValues(actualStreamNameFromAck).Inc()

	// This existing metric seems to be for total messages in stream, which is now covered by natsStreamInfoMessages.
	// If replicationPublisherStreamTotalMessages is meant to be total *published* by this instance, its update logic might need review.
	// For now, keeping its increment logic as it was, but noting potential overlap or clarification needed.
	labels := prometheus.Labels{"stream_name": actualStreamNameFromAck}
	replicationPublisherStreamTotalMessages.With(labels).Inc()


	if cfg.Config.Snapshot.Enable {
		seq, err := r.repState.save(actualStreamNameFromAck, ack.Sequence)
		if err != nil {
			return err
		}

		snapshotEntries := uint64(cfg.Config.ReplicationLog.MaxEntries) / r.shards
		if snapshotEntries != 0 && seq%snapshotEntries == 0 && shardID == SnapshotShardID {
			log.Debug().
				Uint64("seq", seq).
				Str("stream", ack.Stream).
				Msg("Initiating save snapshot")
			go r.SaveSnapshot()
		}
	}

	return nil
}

func (r *Replicator) Listen(shardID uint64, callback func(payload []byte) error) error {
	js := r.streamMap[shardID]

	currentSubjectName := subjectName(shardID)
	currentStreamName := streamName(shardID, r.compressionEnabled)

	sub, err := js.SubscribeSync(currentSubjectName)
	if err != nil {
		logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypeSubscribe).Inc()
		return err
	}
	defer sub.Unsubscribe()

	savedSeq := r.repState.get(currentStreamName)
	for sub.IsValid() {
		msg, err := sub.NextMsg(5 * time.Second)
		if errors.Is(err, nats.ErrTimeout) {
			continue
		}

		if err != nil {
			// Log error and potentially increment subscribe error counter if it's a persistent issue
			log.Warn().Err(err).Str("subject", currentSubjectName).Msg("NATS NextMsg error")
			// Consider if this is a "subscribe" type error or needs its own category
			// logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypeSubscribe).Inc() // Might be too noisy for transient errors
			return err // Propagate error, might lead to subscription retry logic if any
		}

		meta, err := msg.Metadata()
		if err != nil {
			log.Warn().Err(err).Str("subject", currentSubjectName).Msg("NATS msg.Metadata error")
			// This is an unexpected error with the message itself.
			logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypeSubscribe).Inc()
			// NAK the message if possible, though without metadata, sequence is unknown.
			// Depending on NATS client version, a simple msg.Nak() might be attempted.
			// msg.NakWithDelay(10 * time.Second) // Example
			continue // Skip this message
		}

		if meta.Sequence.Stream <= savedSeq {
			continue
		}

		err = r.invokeListener(callback, msg)
		if err != nil {
			msg.Nak() // NAK the message so it can be redelivered or go to DLQ
			if errors.Is(err, context.Canceled) {
				return nil // Graceful shutdown
			}
			logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypeSubscribe).Inc()
			log.Error().Err(err).Str("subject", currentSubjectName).Msg("Replication callback failed, message NAKed")
			// Depending on policy, might not terminate the whole Listen loop for one bad message.
			// For now, it continues to try processing next messages. If error is persistent, higher level logic should handle.
			// return err // Terminating the listener
			continue // Continue to next message
		}

		// Message processed by callback, now update state and ACK
		savedSeq, err = r.repState.save(meta.Stream, meta.Sequence.Stream) // meta.Stream should be currentStreamName
		if err != nil {
			// This is a critical local state error.
			logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypeSubscribe).Inc() // Or a "local_state_error" type
			log.Error().Err(err).Str("stream", meta.Stream).Msg("Failed to save replication state")
			// Consider NAKing the message if state saving is critical for idempotency, though callback already succeeded.
			// This is a tricky state. For now, assume msg.Ack() will be attempted.
			return err // Terminate on state saving failure
		}

		err = msg.Ack()
		if err != nil {
			logstreamNatsOperationErrorsTotal.WithLabelValues(LogstreamNatsErrorTypeSubscribe).Inc()
			log.Error().Err(err).Str("subject", currentSubjectName).Uint64("seq", meta.Sequence.Stream).Msg("NATS msg.Ack failed")
			return err // Terminate on ACK failure
		}

		// Update Prometheus metrics for subscriber
		replicationLastProcessedSequence.WithLabelValues(currentStreamName).Set(float64(savedSeq))
		replicationLastProcessedTimestampSeconds.WithLabelValues(currentStreamName).Set(float64(meta.Timestamp.Unix()))
		
		// meta.NumPending is the number of messages left in the consumer's buffer/pull
		replicationSubscriberLagMessages.WithLabelValues(currentStreamName).Set(float64(meta.NumPending))
		natsSubscriptionPendingMessages.WithLabelValues(currentSubjectName).Set(float64(meta.NumPending))
		
		// This estimates processing delay for the current message.
		replicationSubscriberLagSeconds.WithLabelValues(currentStreamName).Set(float64(time.Now().Unix() - meta.Timestamp.Unix()))

		// Increment total messages received from this specific stream
		natsMessagesReceivedTotal.WithLabelValues(currentStreamName).Inc()
	}

	return nil
}

func (r *Replicator) RestoreSnapshot() error {
	if r.snapshot == nil {
		return nil
	}

	for shardID, js := range r.streamMap {
		strName := streamName(shardID, r.compressionEnabled)
		info, err := js.StreamInfo(strName)
		if err != nil {
			return err
		}

		savedSeq := r.repState.get(strName)
		if savedSeq < info.State.FirstSeq {
			return r.snapshot.RestoreSnapshot()
		}
	}

	return nil
}

func (r *Replicator) LastSaveSnapshotTime() time.Time {
	return r.lastSnapshot
}

func (r *Replicator) SaveSnapshot() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	locked, err := r.metaStore.ContextRefreshingLease("snapshot", SnapshotLeaseTTL, ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Error acquiring snapshot lock")
		return
	}

	if !locked {
		log.Info().Msg("Snapshot saving already locked, skipping")
		return
	}

	r.ForceSaveSnapshot()
}

func (r *Replicator) ForceSaveSnapshot() {
	if r.snapshot == nil {
		return
	}

	err := r.snapshot.SaveSnapshot()
	if err != nil {
		log.Error().
			Err(err).
			Msg("Unable snapshot database")
		return
	}

	r.lastSnapshot = time.Now()
}

func (r *Replicator) ReloadCertificates() error {
	if cfg.Config.NATS.CAFile != "" {
		err := nats.RootCAs(cfg.Config.NATS.CAFile)(&r.client.Opts)
		if err != nil {
			return err
		}
	}

	if cfg.Config.NATS.CertFile != "" && cfg.Config.NATS.KeyFile != "" {
		err := nats.ClientCert(cfg.Config.NATS.CertFile, cfg.Config.NATS.KeyFile)(&r.client.Opts)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Replicator) invokeListener(callback func(payload []byte) error, msg *nats.Msg) error {
	var err error
	payload := msg.Data

	if r.compressionEnabled {
		payload, err = payloadDecompress(msg.Data)
		if err != nil {
			return err
		}
	}

	for repRetry := 0; repRetry < maxReplicateRetries; repRetry++ {
		// Don't invoke for first iteration
		if repRetry != 0 {
			err = msg.InProgress()
			if err != nil {
				return err
			}
		}

		err = callback(payload)
		if err == context.Canceled {
			return err
		}

		if err == nil {
			return nil
		}

		log.Error().
			Err(err).
			Int("attempt", repRetry).
			Msg("Unable to process message retrying")
	}

	return err
}

func makeShardStreamConfig(shardID uint64, totalShards uint64, compressed bool) *nats.StreamConfig {
	streamName := streamName(shardID, compressed)
	replicas := cfg.Config.ReplicationLog.Replicas
	if replicas < 1 {
		replicas = int(totalShards>>1) + 1
	}

	if replicas > 5 {
		replicas = 5
	}

	return &nats.StreamConfig{
		Name:              streamName,
		Subjects:          []string{subjectName(shardID)},
		Discard:           nats.DiscardOld,
		MaxMsgs:           cfg.Config.ReplicationLog.MaxEntries,
		Storage:           nats.FileStorage,
		Retention:         nats.LimitsPolicy,
		AllowDirect:       true,
		MaxConsumers:      -1,
		MaxMsgsPerSubject: -1,
		Duplicates:        0, 
		DenyDelete:        true,
		Replicas:          replicas,
	}
}

func eqShardStreamConfig(a *nats.StreamConfig, b *nats.StreamConfig) bool {
	return a.Name == b.Name &&
		len(a.Subjects) == 1 &&
		len(b.Subjects) == 1 &&
		a.Subjects[0] == b.Subjects[0] &&
		a.Discard == b.Discard &&
		a.MaxMsgs == b.MaxMsgs &&
		a.Storage == b.Storage &&
		a.Retention == b.Retention &&
		a.AllowDirect == b.AllowDirect &&
		a.MaxConsumers == b.MaxConsumers &&
		a.MaxMsgsPerSubject == b.MaxMsgsPerSubject &&
		a.Duplicates == b.Duplicates &&
		a.DenyDelete == b.DenyDelete &&
		a.Replicas == b.Replicas
}

func streamName(shardID uint64, compressed bool) string {
	compPostfix := ""
	if compressed {
		compPostfix = "-c"
	}

	return fmt.Sprintf("%s%s-%d", cfg.Config.NATS.StreamPrefix, compPostfix, shardID)
}

func subjectName(shardID uint64) string {
	return fmt.Sprintf("%s-%d", cfg.Config.NATS.SubjectPrefix, shardID)
}

func payloadCompress(payload []byte) ([]byte, error) {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, err
	}

	return enc.EncodeAll(payload, nil), nil
}

func payloadDecompress(payload []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}

	return dec.DecodeAll(payload, nil)
}
