package logstream

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/db"
)

const (
	schemaNakDelay          = 30 * time.Second
	schemaRecomputeInterval = 5 * time.Minute
)

// ValidateAndReplicateWithSchema validates schema hash and replicates if valid
// Returns true if replication should be retried (schema mismatch), false otherwise
func (r *Replicator) ValidateAndReplicateWithSchema(
	event *db.ChangeLogEvent,
	streamDB *db.SqliteStreamDB,
	msg *nats.Msg,
) (bool, error) {
	// Fast path: hash comparison only (O(1))
	if event.SchemaHash != "" {
		localHash := streamDB.GetSchemaHash()
		prevHash := streamDB.GetPreviousHash()

		// Accept if matches current OR previous schema (for rolling upgrades)
		if event.SchemaHash != localHash && event.SchemaHash != prevHash {
			return r.handleSchemaMismatch(event, streamDB, msg, localHash)
		}
	}

	// Hashes match (or no hash in event) - apply directly
	r.resetMismatchState()
	return false, streamDB.Replicate(event)
}

// handleSchemaMismatch handles schema mismatch by NAKing the message and optionally recomputing
func (r *Replicator) handleSchemaMismatch(
	event *db.ChangeLogEvent,
	streamDB *db.SqliteStreamDB,
	msg *nats.Msg,
	localHash string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	if r.schemaMismatchAt.IsZero() {
		// First mismatch - record timestamp, recompute immediately
		r.schemaMismatchAt = now
		r.lastRecomputeAt = now
		r.schemaMismatchMetric.Set(1)

		ctx := context.Background()
		newHash, err := streamDB.GetSchemaCache().Recompute(ctx)
		if err == nil {
			prevHash := streamDB.GetPreviousHash()
			// Check if event matches current or previous hash after recompute
			if event.SchemaHash == newHash || event.SchemaHash == prevHash {
				// Schema matches after recompute (e.g., DDL applied before startup)
				log.Info().Msg("Schema matches after initial recompute, applying event")
				r.resetMismatchStateLocked()
				return false, streamDB.Replicate(event)
			}
		}

		log.Warn().
			Str("event_hash", truncateHash(event.SchemaHash)).
			Str("local_hash", truncateHash(newHash)).
			Msg("Schema mismatch detected, pausing replication")

	} else if now.Sub(r.lastRecomputeAt) >= schemaRecomputeInterval {
		// We've been paused for a while - recompute to check if DDL was applied
		r.lastRecomputeAt = now

		// Check for stream gap before recomputing schema
		if r.checkStreamGap() {
			log.Fatal().
				Dur("paused_for", now.Sub(r.schemaMismatchAt)).
				Msg("Stream gap detected during schema mismatch pause, exiting for snapshot restore")
			// Process exits here. On restart, RestoreSnapshot() will run.
		}

		ctx := context.Background()
		newHash, err := streamDB.GetSchemaCache().Recompute(ctx)
		if err == nil {
			prevHash := streamDB.GetPreviousHash()
			// Check if event matches current or previous hash after recompute
			if event.SchemaHash == newHash || event.SchemaHash == prevHash {
				// Schema now matches after local DDL was applied
				log.Info().
					Dur("paused_for", now.Sub(r.schemaMismatchAt)).
					Msg("Schema now matches after recompute, resuming replication")
				r.resetMismatchStateLocked()
				return false, streamDB.Replicate(event)
			}
		}

		log.Warn().
			Str("event_hash", truncateHash(event.SchemaHash)).
			Str("local_hash", truncateHash(newHash)).
			Dur("paused_for", now.Sub(r.schemaMismatchAt)).
			Msg("Schema still mismatched after recompute")
	}

	// Still mismatched - NAK and wait
	msg.NakWithDelay(schemaNakDelay)
	return true, nil
}

// checkStreamGap returns true if any stream has truncated messages we need
func (r *Replicator) checkStreamGap() bool {
	for shardID, js := range r.streamMap {
		strName := streamName(shardID, r.compressionEnabled)
		info, err := js.StreamInfo(strName)
		if err != nil {
			continue
		}

		savedSeq := r.repState.get(strName)
		if savedSeq < info.State.FirstSeq {
			log.Warn().
				Str("stream", strName).
				Uint64("saved_seq", savedSeq).
				Uint64("first_seq", info.State.FirstSeq).
				Msg("Stream gap detected: required messages have been truncated")
			return true
		}
	}
	return false
}

// resetMismatchState resets schema mismatch tracking (thread-safe)
func (r *Replicator) resetMismatchState() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resetMismatchStateLocked()
}

// resetMismatchStateLocked resets mismatch state (caller must hold lock)
func (r *Replicator) resetMismatchStateLocked() {
	r.schemaMismatchAt = time.Time{}
	r.lastRecomputeAt = time.Time{}
	r.schemaMismatchMetric.Set(0)
}

// IsSchemaMismatchPaused returns true if replication is paused due to schema mismatch
func (r *Replicator) IsSchemaMismatchPaused() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.schemaMismatchAt.IsZero()
}

// truncateHash returns first 8 characters of hash for logging
func truncateHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
