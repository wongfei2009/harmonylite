package db

import (
	"context"
	"fmt"
	"sync"
)

// SchemaCache holds the precomputed schema hash for fast validation
type SchemaCache struct {
	mu            sync.RWMutex
	schemaHash    string
	previousHash  string // Hash before last schema change (for rolling upgrades)
	schemaManager *SchemaManager
	tables        []string
}

// NewSchemaCache creates a new SchemaCache
func NewSchemaCache() *SchemaCache {
	return &SchemaCache{}
}

// Initialize computes and caches the schema hash for watched tables
func (sc *SchemaCache) Initialize(ctx context.Context, sm *SchemaManager, tables []string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	hash, err := sm.ComputeSchemaHash(ctx, tables)
	if err != nil {
		return fmt.Errorf("computing schema hash: %w", err)
	}
	sc.schemaHash = hash
	sc.schemaManager = sm
	sc.tables = tables
	return nil
}

// GetSchemaHash returns the cached schema hash (O(1))
func (sc *SchemaCache) GetSchemaHash() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.schemaHash
}

// GetPreviousHash returns the previous schema hash (O(1))
// Used during rolling upgrades to accept events from nodes not yet upgraded
func (sc *SchemaCache) GetPreviousHash() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.previousHash
}

// Recompute recalculates the schema hash from the database
// Called during pause state to detect if local DDL has been applied
// When schema changes, the old hash is preserved as previousHash
func (sc *SchemaCache) Recompute(ctx context.Context) (string, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	hash, err := sc.schemaManager.ComputeSchemaHash(ctx, sc.tables)
	if err != nil {
		return "", fmt.Errorf("recomputing schema hash: %w", err)
	}

	// Preserve old hash as previous when schema changes
	if hash != sc.schemaHash && sc.schemaHash != "" {
		sc.previousHash = sc.schemaHash
	}
	sc.schemaHash = hash
	return hash, nil
}

// IsInitialized returns true if the cache has been initialized
func (sc *SchemaCache) IsInitialized() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.schemaManager != nil && len(sc.tables) > 0
}
