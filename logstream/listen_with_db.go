package logstream

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/db"
)

// ListenWithDB listens for replication events with schema validation support
func (r *Replicator) ListenWithDB(shardID uint64, streamDB *db.SqliteStreamDB, callback func(event *db.ChangeLogEvent) error) error {
	js := r.streamMap[shardID]

	sub, err := js.SubscribeSync(subjectName(shardID))
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	savedSeq := r.repState.get(streamName(shardID, r.compressionEnabled))
	for sub.IsValid() {
		msg, err := sub.NextMsg(5 * time.Second)
		if errors.Is(err, nats.ErrTimeout) {
			continue
		}

		if err != nil {
			return err
		}

		meta, err := msg.Metadata()
		if err != nil {
			return err
		}

		if meta.Sequence.Stream <= savedSeq {
			continue
		}

		err = r.invokeListenerWithSchemaValidation(streamDB, callback, msg)
		if err != nil {
			msg.Nak()
			if errors.Is(err, context.Canceled) {
				return nil
			}

			log.Error().Err(err).Msg("Replication failed, terminating...")
			return err
		}

		savedSeq, err = r.repState.save(meta.Stream, meta.Sequence.Stream)
		if err != nil {
			return err
		}

		err = msg.Ack()
		if err != nil {
			return err
		}
	}

	return nil
}

// invokeListenerWithSchemaValidation unpacks the event, validates schema, and invokes the callback
func (r *Replicator) invokeListenerWithSchemaValidation(streamDB *db.SqliteStreamDB, callback func(event *db.ChangeLogEvent) error, msg *nats.Msg) error {
	var err error
	payload := msg.Data

	if r.compressionEnabled {
		payload, err = payloadDecompress(msg.Data)
		if err != nil {
			return err
		}
	}

	// Unmarshal the replication event
	ev := &ReplicationEvent[db.ChangeLogEvent]{}
	err = ev.Unmarshal(payload)
	if err != nil {
		return err
	}

	// Validate schema hash
	shouldRetry, err := r.ValidateAndReplicateWithSchema(&ev.Payload, streamDB, msg)
	if shouldRetry {
		// Schema mismatch - message was NAK'd, will be redelivered
		return nil
	}
	if err != nil {
		return err
	}

	// Call the user callback (for any additional processing)
	return callback(&ev.Payload)
}
