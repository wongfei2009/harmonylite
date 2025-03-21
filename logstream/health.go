package logstream

import (
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

// IsConnected checks if the NATS connection is alive
func (r *Replicator) IsConnected() bool {
	if r.client == nil {
		return false
	}
	
	return r.client.Status() == nats.CONNECTED
}

// GetLastReplicatedEventTime returns an approximate time of the last replicated event
// This is a simplified implementation that could be enhanced with actual tracking
func (r *Replicator) GetLastReplicatedEventTime() time.Time {
	// Check if we have any stream information
	if len(r.streamMap) == 0 {
		return time.Time{}
	}
	
	// For simplicity, we'll check just the first shard
	js, ok := r.streamMap[1]
	if !ok {
		return time.Time{}
	}
	
	// Get the stream info to find the last sequence
	info, err := js.StreamInfo(streamName(1, r.compressionEnabled))
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get stream info for last replicated event time")
		return time.Time{}
	}
	
	// If there are no messages, return zero time
	if info.State.LastSeq == 0 {
		return time.Time{}
	}
	
	// Try to get the last message's timestamp
	// Note: This is not exact, as it's just checking one shard
	msg, err := js.GetMsg(streamName(1, r.compressionEnabled), info.State.LastSeq)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get last message for timestamp")
		return time.Time{}
	}
	
	return msg.Time
}

// GetLastPublishedEventTime returns the time of the last published event
// This is similar to GetLastReplicatedEventTime but for outgoing messages
func (r *Replicator) GetLastPublishedEventTime() time.Time {
	// For the initial implementation, we'll use the same logic as GetLastReplicatedEventTime
	// In a more detailed implementation, we would track separate timestamps for publishing vs. replicating
	return r.GetLastReplicatedEventTime()
}
