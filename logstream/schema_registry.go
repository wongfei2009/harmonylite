package logstream

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/version"
)

const SchemaRegistryBucket = "harmonylite-schema-registry"

// NodeSchemaState represents the schema state of a single node in the cluster
type NodeSchemaState struct {
	NodeId             uint64    `json:"node_id"`
	SchemaHash         string    `json:"schema_hash"`
	HarmonyLiteVersion string    `json:"harmonylite_version"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// SchemaMismatch represents a schema inconsistency between nodes
type SchemaMismatch struct {
	NodeId       uint64 `json:"node_id"`
	ExpectedHash string `json:"expected_hash"`
	ActualHash   string `json:"actual_hash"`
}

// SchemaConsistencyReport summarizes cluster-wide schema consistency
type SchemaConsistencyReport struct {
	Timestamp  time.Time        `json:"timestamp"`
	NodeCount  int              `json:"node_count"`
	Consistent bool             `json:"consistent"`
	Mismatches []SchemaMismatch `json:"mismatches,omitempty"`
}

// SchemaRegistry provides cluster-wide schema state visibility via NATS KV
type SchemaRegistry struct {
	nodeID uint64
	kv     nats.KeyValue
}

// NewSchemaRegistry creates a new schema registry using the provided NATS connection
// It creates the KV bucket if it doesn't exist
func NewSchemaRegistry(nc *nats.Conn, nodeID uint64) (*SchemaRegistry, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("creating jetstream context: %w", err)
	}

	// Create or get the KV bucket
	kv, err := js.KeyValue(SchemaRegistryBucket)
	if err == nats.ErrBucketNotFound {
		log.Debug().Str("bucket", SchemaRegistryBucket).Msg("Creating schema registry bucket")
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:      SchemaRegistryBucket,
			Description: "HarmonyLite schema registry for cluster-wide schema visibility",
			TTL:         5 * time.Minute, // Expire entries after 5 minutes of no updates
			Replicas:    1,               // Single replica for now
		})
	}
	if err != nil {
		return nil, fmt.Errorf("getting/creating schema registry bucket: %w", err)
	}

	return &SchemaRegistry{
		nodeID: nodeID,
		kv:     kv,
	}, nil
}

// PublishSchemaState publishes the current node's schema state to the registry
func (sr *SchemaRegistry) PublishSchemaState(schemaHash string) error {
	state := NodeSchemaState{
		NodeId:             sr.nodeID,
		SchemaHash:         schemaHash,
		HarmonyLiteVersion: version.Get().Version,
		UpdatedAt:          time.Now(),
	}

	key := fmt.Sprintf("node-%d", sr.nodeID)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling schema state: %w", err)
	}

	_, err = sr.kv.Put(key, data)
	if err != nil {
		return fmt.Errorf("publishing schema state to KV: %w", err)
	}

	hashPreview := schemaHash
	if len(schemaHash) > 16 {
		hashPreview = schemaHash[:16]
	}

	log.Debug().
		Uint64("node_id", sr.nodeID).
		Str("schema_hash", hashPreview).
		Msg("Published schema state to registry")

	return nil
}

// GetClusterSchemaState retrieves schema state for all nodes in the cluster
func (sr *SchemaRegistry) GetClusterSchemaState() (map[uint64]*NodeSchemaState, error) {
	states := make(map[uint64]*NodeSchemaState)

	keys, err := sr.kv.Keys()
	if err != nil && err != nats.ErrNoKeysFound {
		return nil, fmt.Errorf("listing keys from KV: %w", err)
	}

	for _, key := range keys {
		entry, err := sr.kv.Get(key)
		if err != nil {
			log.Warn().Str("key", key).Err(err).Msg("Failed to get key from schema registry")
			continue
		}

		var state NodeSchemaState
		if err := json.Unmarshal(entry.Value(), &state); err != nil {
			log.Warn().Str("key", key).Err(err).Msg("Failed to unmarshal schema state")
			continue
		}

		states[state.NodeId] = &state
	}

	return states, nil
}

// CheckClusterSchemaConsistency checks if all nodes in the cluster have consistent schemas
func (sr *SchemaRegistry) CheckClusterSchemaConsistency() (*SchemaConsistencyReport, error) {
	states, err := sr.GetClusterSchemaState()
	if err != nil {
		return nil, err
	}

	report := &SchemaConsistencyReport{
		Timestamp:  time.Now(),
		NodeCount:  len(states),
		Consistent: true,
		Mismatches: []SchemaMismatch{},
	}

	// If no nodes or only one node, consider it consistent
	if len(states) <= 1 {
		return report, nil
	}

	// Use the first node's hash as the reference
	var referenceHash string
	for _, state := range states {
		referenceHash = state.SchemaHash
		break
	}

	// Compare all other nodes against the reference
	for nodeID, state := range states {
		if state.SchemaHash != referenceHash {
			report.Consistent = false
			report.Mismatches = append(report.Mismatches, SchemaMismatch{
				NodeId:       nodeID,
				ExpectedHash: referenceHash,
				ActualHash:   state.SchemaHash,
			})
		}
	}

	return report, nil
}

// GetNodeSchemaState retrieves the schema state for a specific node
func (sr *SchemaRegistry) GetNodeSchemaState(nodeID uint64) (*NodeSchemaState, error) {
	key := fmt.Sprintf("node-%d", nodeID)
	entry, err := sr.kv.Get(key)
	if err != nil {
		return nil, fmt.Errorf("getting node schema state: %w", err)
	}

	var state NodeSchemaState
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		return nil, fmt.Errorf("unmarshaling schema state: %w", err)
	}

	return &state, nil
}
