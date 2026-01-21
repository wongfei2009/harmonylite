# Schema Versioning and Migration Design

**Status:** Implemented
**Author:** TBD
**Created:** 2025-01-17

## Overview

This document proposes a schema versioning and migration handling system for HarmonyLite to address scenarios where database instances in a cluster have different schema versions during rolling upgrades or partial migrations.

## Problem Statement

### Current Behavior

HarmonyLite currently has **no schema versioning or mismatch detection**. When schemas differ between nodes:

| Scenario | Current Behavior |
|----------|------------------|
| Missing column on target | `INSERT OR REPLACE` fails, retries 7x, replication stops |
| Extra column with default | Silent inconsistency (uses SQLite default) |
| Type mismatch | SQLite allows it (weak typing), but comparison/indexing may behave unexpectedly |
| Table doesn't exist | Replication fails with error |

### Impact

- **Silent data inconsistency** across nodes
- **Replication failures** that are difficult to diagnose
- **Manual coordination required** for schema changes (stop all nodes, apply changes, restart)
- **No visibility** into cluster schema state

## Goals

1. **Detect** schema mismatches before they cause replication failures
2. **Provide visibility** into cluster-wide schema state
3. **Pause replication safely** during rolling upgrades until schema versions converge

## Non-Goals

- Automatic schema migration (DDL replication)
- Distributed transactions
- Strong consistency guarantees
- Schema diff between nodes (users can compare hashes manually)
- Coordinated migration protocol (users apply DDL manually per node)

---

## Atlas Integration

This design leverages [Atlas](https://atlasgo.io/) (`ariga.io/atlas`) as a library for schema introspection and comparison. Atlas is a mature, MIT-licensed database schema management tool with excellent SQLite support.

### Why Atlas?

| Consideration | Decision |
|---------------|----------|
| **Schema Introspection** | Atlas provides battle-tested SQLite introspection via `PRAGMA table_info`, `PRAGMA index_list`, etc. with proper handling of edge cases (quoted identifiers, generated columns, partial indexes) |
| **Deterministic Hashing** | Atlas's `migrate.HashFile` uses a well-defined cumulative SHA-256 algorithm that we can adapt |
| **Schema Comparison** | Atlas's `Differ` interface handles SQLite-specific normalization (e.g., `INTEGER` vs `INT`) |
| **Maintenance Burden** | Using Atlas as a library means we benefit from upstream fixes without maintaining our own introspection code |
| **License** | MIT license is compatible with HarmonyLite's license |

### What We Use from Atlas

We use Atlas **as a library only**, not as a CLI or migration engine:

```go
import (
    "ariga.io/atlas/sql/sqlite"
    "ariga.io/atlas/sql/schema"
)
```

| Atlas Component | Our Usage |
|-----------------|-----------|
| `sqlite.Driver` | Schema introspection via `InspectRealm()` |
| `schema.Table`, `schema.Column` | Data structures for representing schema |
| Hashing algorithm | Adapted for deterministic schema hashing |

### What We Don't Use

- Atlas CLI commands
- Atlas migration engine (`migrate.Executor`)
- Atlas HCL configuration files
- Atlas schema registry / versioning (we implement our own via NATS KV)
- Atlas `Differ` for schema comparison (out of scope)

---

## Proposed Design

### 1. Schema Tracking

Add a schema tracking table to each node:

```sql
CREATE TABLE IF NOT EXISTS __harmonylite__schema_version (
    id INTEGER PRIMARY KEY CHECK (id = 1),  -- Single row constraint
    schema_hash TEXT NOT NULL,              -- Hash of watched/replicated tables
    updated_at INTEGER NOT NULL,
    harmonylite_version TEXT
);
```

**Fields:**
- `schema_hash`: SHA-256 hash of replicated/watched table schemas (deterministic)
- `updated_at`: Unix timestamp of last schema state update
- `harmonylite_version`: HarmonyLite binary version that recorded the schema state

### 2. Schema Hash Calculation

Implement deterministic schema hashing using Atlas for introspection:

```go
// db/schema_manager.go

import (
    "context"
    "crypto/sha256"
    "database/sql"
    "encoding/hex"
    "fmt"
    "sort"

    "ariga.io/atlas/sql/schema"
    "ariga.io/atlas/sql/sqlite"
)

// SchemaManager wraps Atlas's SQLite driver for schema operations
type SchemaManager struct {
    driver *sqlite.Driver
    db     *sql.DB
}

// NewSchemaManager creates a new SchemaManager using the provided database connection
func NewSchemaManager(db *sql.DB) (*SchemaManager, error) {
    // Open Atlas driver on existing connection
    driver, err := sqlite.Open(db)
    if err != nil {
        return nil, fmt.Errorf("opening atlas driver: %w", err)
    }
    return &SchemaManager{driver: driver, db: db}, nil
}

// InspectTables returns Atlas schema.Table objects for the specified tables
func (sm *SchemaManager) InspectTables(ctx context.Context, tables []string) ([]*schema.Table, error) {
    // Inspect the schema realm (all tables)
    realm, err := sm.driver.InspectRealm(ctx, &schema.InspectRealmOption{
        Schemas: []string{"main"},
    })
    if err != nil {
        return nil, fmt.Errorf("inspecting realm: %w", err)
    }

    if len(realm.Schemas) == 0 {
        return nil, nil
    }

    // Filter to requested tables
    tableSet := make(map[string]bool)
    for _, t := range tables {
        tableSet[t] = true
    }

    var result []*schema.Table
    for _, t := range realm.Schemas[0].Tables {
        if tableSet[t.Name] {
            result = append(result, t)
        }
    }
    return result, nil
}

// ComputeSchemaHash computes a deterministic SHA-256 hash of the specified tables
func (sm *SchemaManager) ComputeSchemaHash(ctx context.Context, tables []string) (string, error) {
    inspected, err := sm.InspectTables(ctx, tables)
    if err != nil {
        return "", err
    }

    // Sort tables by name for determinism
    sort.Slice(inspected, func(i, j int) bool {
        return inspected[i].Name < inspected[j].Name
    })

    h := sha256.New()
    for _, table := range inspected {
        if err := hashTable(h, table); err != nil {
            return "", err
        }
    }

    return hex.EncodeToString(h.Sum(nil)), nil
}

// hashTable writes a deterministic representation of a table to the hasher
func hashTable(h io.Writer, table *schema.Table) error {
    // Sort columns by name for determinism
    cols := make([]*schema.Column, len(table.Columns))
    copy(cols, table.Columns)
    sort.Slice(cols, func(i, j int) bool {
        return cols[i].Name < cols[j].Name
    })

    // Write table name
    h.Write([]byte(table.Name))

    // Write each column: |name:type:notnull:pk
    for _, col := range cols {
        isPK := false
        if table.PrimaryKey != nil {
            for _, pkCol := range table.PrimaryKey.Parts {
                if pkCol.C != nil && pkCol.C.Name == col.Name {
                    isPK = true
                    break
                }
            }
        }

        // Normalize type string using Atlas's type representation
        typeStr := col.Type.Raw
        if typeStr == "" && col.Type.Type != nil {
            typeStr = fmt.Sprintf("%T", col.Type.Type)
        }

        h.Write([]byte(fmt.Sprintf("|%s:%s:%t:%t",
            col.Name, typeStr, !col.Type.Null, isPK)))
    }
    h.Write([]byte("\n"))
    return nil
}
```

**Key Design Decisions:**

1. **Atlas for Introspection**: We use Atlas's `InspectRealm()` which handles SQLite edge cases (quoted identifiers, generated columns, etc.)

2. **Hex-encoded SHA-256**: We use hex encoding (not base64) to match common conventions and be easier to read/compare in logs

3. **Deterministic Ordering**: Tables and columns are sorted alphabetically before hashing

4. **Type Normalization**: Atlas normalizes SQLite types (e.g., `INT` → `INTEGER`) ensuring consistent hashes across nodes even if DDL was written differently

### 3. Extended Replication Event Format

Include schema metadata in replication events:

```go
// db/change_log.go

type ChangeLogEvent struct {
    // Existing fields
    Id        int64
    Type      string           // "insert" | "update" | "delete"
    TableName string
    Row       map[string]any

    // New field for schema tracking
    SchemaHash string `cbor:"sh,omitempty"`  // Hash of all watched tables at creation
}
```

**Backward Compatibility:**
- Use CBOR `omitempty` tags
- Old events without `SchemaHash` are treated as "unknown schema"

### 4. Schema Hash Caching and Validation

To avoid expensive per-event validation, the schema hash is computed once and cached:

```go
// db/schema_cache.go

// SchemaCache holds the precomputed schema hash for fast validation
type SchemaCache struct {
    mu            sync.RWMutex
    schemaHash    string
    previousHash  string           // Hash before last schema change (for rolling upgrades)
    schemaManager *SchemaManager
    tables        []string
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
```

**Per-Event Validation:**

The validation in the replication hot path is a simple string comparison:

```go
// logstream/replicator.go

func (r *Replicator) validateAndApplyEvent(event *ChangeLogEvent, msg *nats.Msg) error {
    // Fast path: hash comparison only (O(1))
    if event.SchemaHash != "" {
        localHash := r.schemaCache.GetSchemaHash()
        prevHash := r.schemaCache.GetPreviousHash()
        
        // Accept if matches current OR previous schema (for rolling upgrades)
        if event.SchemaHash != localHash && event.SchemaHash != prevHash {
            return r.handleSchemaMismatch(event, msg)
        }
    }

    // Hashes match (or no hash in event) - apply directly
    r.resetMismatchState()
    return r.db.ReplicateRow(event)
}
```

**Performance Characteristics:**

| Operation | Cost | When |
|-----------|------|------|
| Hash computation | O(tables + columns) + PRAGMA calls | On startup, or during pause recompute interval |
| Per-event validation | O(1) string comparison (x2) | Every incoming event |
| Cache lookup | O(1) string read with RLock | Every incoming event |

**Rolling Upgrade Support:**

The `previousHash` field enables smooth rolling upgrades when multiple nodes have `publish=true`. Consider this scenario:

1. **Node A** upgrades first → computes new schema hash `H2`, preserves `H1` as `previousHash`
2. **Node B** hasn't upgraded yet → continues publishing events with hash `H1`
3. **Node A** receives events from Node B with hash `H1`
4. Since `H1` matches `previousHash`, Node A accepts the event without pausing

This handles the common case where schema changes are backward compatible (e.g., adding a nullable column). Events from the previous schema version can still be applied to the new schema.

**Limitation:** Only one previous version is tracked. If Node B is two or more schema versions behind, replication will pause until Node B catches up.

**Cache Recomputation:**

The cache is computed:
- On HarmonyLite startup
- When running `harmonylite -cleanup`
- Periodically during schema mismatch pause state (see Section 5)

### 5. Schema Mismatch Handling (Pause with Periodic Recompute)

When a schema mismatch is detected (hash comparison fails), replication pauses for that shard by NAK-ing the message with a delay. The sequence map is not advanced, so ordering is preserved.

**Key Behavior:** During the pause state, HarmonyLite periodically recomputes the local schema hash to detect if DDL has been applied locally. Once schemas match, replication resumes automatically without requiring a restart.

```go
// logstream/replicator.go

type Replicator struct {
    // ... existing fields
    schemaCache        *SchemaCache
    schemaMismatchAt   time.Time     // When mismatch first detected
    lastRecomputeAt    time.Time     // Last time we recomputed during pause
}

const (
    schemaNakDelay          = 30 * time.Second
    schemaRecomputeInterval = 5 * time.Minute
)

func (r *Replicator) handleSchemaMismatch(event *ChangeLogEvent, msg *nats.Msg) error {
    now := time.Now()

    if r.schemaMismatchAt.IsZero() {
        // First mismatch - record timestamp, recompute immediately
        r.schemaMismatchAt = now
        r.lastRecomputeAt = now

        newHash, err := r.schemaCache.Recompute(context.Background())
        if err == nil && event.SchemaHash == newHash {
            // Schema matches after recompute (e.g., DDL applied before startup)
            log.Info().Msg("Schema matches after initial recompute, applying event")
            r.resetMismatchState()
            return r.db.ReplicateRow(event)
        }

        log.Warn().
            Str("event_hash", event.SchemaHash[:8]).
            Str("local_hash", newHash[:8]).
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

        newHash, err := r.schemaCache.Recompute(context.Background())
        if err == nil && event.SchemaHash == newHash {
            // Schema now matches after local DDL was applied
            log.Info().
                Dur("paused_for", now.Sub(r.schemaMismatchAt)).
                Msg("Schema now matches after recompute, resuming replication")
            r.resetMismatchState()
            return r.db.ReplicateRow(event)
        }

        log.Warn().
            Str("event_hash", event.SchemaHash[:8]).
            Str("local_hash", newHash[:8]).
            Dur("paused_for", now.Sub(r.schemaMismatchAt)).
            Msg("Schema still mismatched after recompute")
    }

    // Still mismatched - NAK and wait
    msg.NakWithDelay(schemaNakDelay)
    return nil
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

func (r *Replicator) resetMismatchState() {
    r.schemaMismatchAt = time.Time{}
    r.lastRecomputeAt = time.Time{}
}
```

**Behavior Summary:**

| State | Action |
|-------|--------|
| First mismatch | Recompute hash immediately, NAK if still mismatched |
| Subsequent NAKs within 5 min | Just NAK (no recompute) |
| After 5 min pause | Check for stream gap, then recompute hash |
| Stream gap detected | Exit process for snapshot restore on restart |
| Schema matches after recompute | Resume replication immediately |
| Schema still mismatched | Log warning with pause duration, continue waiting |

**Self-Healing After DDL:**

Once DDL is applied locally (e.g., `ALTER TABLE users ADD COLUMN email TEXT`), the next recompute cycle will detect the schema change and resume replication automatically. No restart or manual intervention is required.

**Stream Gap Detection:**

If replication stays paused long enough for JetStream to truncate messages (due to `MaxMsgs` limit), the node will detect this during the periodic recompute check and exit:

1. Every 5 minutes during pause, `checkStreamGap()` compares `savedSeq` against `stream.FirstSeq`
2. If `savedSeq < FirstSeq`, required messages have been truncated
3. HarmonyLite logs a fatal error and exits
4. On restart, the existing `RestoreSnapshot()` logic downloads a snapshot and restores the database
5. Replication resumes from the snapshot state

This ensures nodes don't get stuck in an unrecoverable state during prolonged schema mismatches.

**Performance During Pause:**

- NAK redelivery: every 30 seconds
- Hash recomputation: every 5 minutes (not every NAK cycle)
- This minimizes database introspection overhead during prolonged migration windows

### 6. Schema Registry via NATS KV

Broadcast schema state across the cluster using NATS KeyValue:

```go
// logstream/schema_registry.go

const SchemaRegistryBucket = "harmonylite-schema-registry"

type NodeSchemaState struct {
    NodeId             uint64            `json:"node_id"`
    SchemaHash         string            `json:"schema_hash"`          // Current hash of all watched tables
    PreviousHash       string            `json:"previous_hash"`        // Previous hash (for rolling upgrade visibility)
    HarmonyLiteVersion string            `json:"harmonylite_version"`
    UpdatedAt          time.Time         `json:"updated_at"`
}

func (r *Replicator) PublishSchemaState() error {
    state := NodeSchemaState{
        NodeId:             r.nodeId,
        SchemaHash:         r.schemaCache.GetSchemaHash(),
        PreviousHash:       r.schemaCache.GetPreviousHash(),
        HarmonyLiteVersion: version.Version,
        UpdatedAt:          time.Now(),
    }

    key := fmt.Sprintf("node-%d", r.nodeId)
    data, _ := json.Marshal(state)

    _, err := r.schemaKV.Put(key, data)
    return err
}

func (r *Replicator) GetClusterSchemaState() (map[uint64]*NodeSchemaState, error) {
    states := make(map[uint64]*NodeSchemaState)

    keys, _ := r.schemaKV.Keys()
    for _, key := range keys {
        entry, err := r.schemaKV.Get(key)
        if err != nil {
            continue
        }
        var state NodeSchemaState
        json.Unmarshal(entry.Value(), &state)
        states[state.NodeId] = &state
    }

    return states, nil
}

func (r *Replicator) CheckClusterSchemaConsistency() (*SchemaConsistencyReport, error) {
    states, err := r.GetClusterSchemaState()
    if err != nil {
        return nil, err
    }

    report := &SchemaConsistencyReport{
        Timestamp:  time.Now(),
        NodeCount:  len(states),
        Consistent: true,
    }

    var referenceHash string
    for nodeId, state := range states {
        if referenceHash == "" {
            referenceHash = state.SchemaHash
        } else if state.SchemaHash != referenceHash {
            report.Consistent = false
            report.Mismatches = append(report.Mismatches, SchemaMismatch{
                NodeId:       nodeId,
                ExpectedHash: referenceHash,
                ActualHash:   state.SchemaHash,
            })
        }
    }

    return report, nil
}
```

### 7. CLI Commands

Add schema management commands:

```bash
# Check schema status (local node)
harmonylite schema status
# Output:
# Local Schema Status
# ===================
# Schema Hash: f6e5d4c3b2a1...
# Previous Hash: a1b2c3d4e5f6... (accepted during rolling upgrades)
# Updated At: 2025-01-17 10:30:00
# HarmonyLite Version: 1.2.0
#
# Watched Tables:
#   users
#   orders

# Check schema status with cluster view (requires NATS connection)
harmonylite schema status --cluster
# Output:
# Cluster Schema Status
# =====================
# Total Nodes: 3
# Consistent: No (rolling upgrade in progress)
#
# Hash Groups:
#   a1b2c3d4: Node 1, Node 2 (current)
#   e5f6g7h8: Node 3 (current), Node 1 (previous), Node 2 (previous)

```

---

## Implementation Phases

### Phase 1: Foundation (Schema Tracking with Atlas)
- [ ] Add `ariga.io/atlas` dependency to `go.mod`
- [ ] Create `db/schema_manager.go` with `SchemaManager` type wrapping Atlas SQLite driver
- [ ] Implement `InspectTables()` and `ComputeSchemaHash()`
- [ ] Create `db/schema_cache.go` with `SchemaCache` type for caching current and previous schema hash
- [ ] Add `__harmonylite__schema_version` table creation in `InstallCDC()`
- [ ] Initialize schema cache on startup
- [ ] Preserve previous hash when schema changes during `Recompute()`
- [ ] Update schema state on `-cleanup` command (invalidate and recompute cache)
- [ ] Add `harmonylite schema status` CLI command (local only)

### Phase 2: Event Enhancement
- [ ] Add `SchemaHash` field to `ChangeLogEvent`
- [ ] Populate field during event creation using cached hash (O(1) lookup)
- [ ] Ensure backward compatibility with old events (CBOR `omitempty`)

### Phase 3: Validation and Pause-on-Mismatch
- [ ] Add `schemaMismatchAt` and `lastRecomputeAt` fields to `Replicator`
- [ ] Implement `handleSchemaMismatch()` with periodic recompute logic
- [ ] Add hash comparison in replication hot path (check current and previous hash)
- [ ] NAK with delay when schema hash mismatches both current and previous
- [ ] Recompute hash on first mismatch and every 5 minutes during pause
- [ ] Implement `checkStreamGap()` to detect truncated messages during pause
- [ ] Exit process when stream gap detected (triggers snapshot restore on restart)
- [ ] Auto-resume when schema matches after recompute
- [ ] Add `harmonylite_schema_mismatch_paused` gauge metric

### Phase 4: Cluster Visibility
- [ ] Create NATS KV bucket `harmonylite-schema-registry`
- [ ] Create `logstream/schema_registry.go` with registry client
- [ ] Implement `PublishSchemaState()` on startup and schema change
- [ ] Implement `GetClusterSchemaState()` and `CheckClusterSchemaConsistency()`
- [ ] Update `harmonylite schema status --cluster` to show cluster view

---

## Constants

The following constants are used:

| Constant | Value | Description |
|----------|-------|-------------|
| `schemaRecomputeInterval` | `5m` | How often to recompute schema hash during pause state |
| `schemaNakDelay` | `30s` | Delay before NATS redelivers a NAK'd message |

---

## Metrics and Observability

### Prometheus Metrics

```
# Schema hash on this node (for alerting on changes)
harmonylite_schema_hash_info{node_id="1", hash="a1b2c3d4"} 1

# Replication paused due to schema mismatch (1 = paused, 0 = normal)
harmonylite_schema_mismatch_paused 1
```

The `harmonylite_schema_mismatch_paused` gauge is the primary metric for troubleshooting. When set to 1, check logs for hash details and apply DDL to the local node.

### Health Check Extension

Extend existing health check endpoint:

```json
{
  "status": "degraded",
  "checks": {
    "schema": {
      "status": "warning",
      "schema_hash": "a1b2c3d4e5f6",
      "paused": true
    }
  }
}
```

---

## Migration Guide

### Performing Schema Migrations

Schema migrations are performed manually on each node. HarmonyLite supports **rolling upgrades** by accepting events from both the current and previous schema versions. **No restart is required** - HarmonyLite automatically detects schema changes during the pause state.

#### Rolling Upgrade Scenario (Multiple Publishers)

When multiple nodes have `publish=true`, the previous hash tracking ensures smooth upgrades:

```bash
# 1. Apply DDL on Node A (first node to upgrade)
sqlite3 mydb.db "ALTER TABLE users ADD COLUMN email TEXT"

# Node A now has:
#   - current_hash: H2 (new schema)
#   - previous_hash: H1 (old schema)

# 2. Node B hasn't upgraded yet, still publishing events with hash H1
#    Node A receives these events and accepts them (H1 matches previous_hash)

# 3. Apply DDL on Node B
sqlite3 mydb.db "ALTER TABLE users ADD COLUMN email TEXT"

# Node B now has:
#   - current_hash: H2
#   - previous_hash: H1

# 4. All nodes now have matching current_hash, cluster is consistent
```

#### Standard Migration (Single Publisher or Sequential)

```bash
# 1. Apply DDL on Node 1
sqlite3 mydb.db "ALTER TABLE users ADD COLUMN email TEXT"

# 2. HarmonyLite detects the schema change automatically (within 5 minutes)
#    No restart required! Replication resumes once schema matches.

# 3. Repeat for other nodes
# During migration window, nodes with older schemas will pause replication

# 4. After all nodes have the new schema, replication resumes automatically
```

**Optional: Force Immediate Detection**

If you don't want to wait for the 5-minute recompute interval:

```bash
# Option A: Restart HarmonyLite (schema hash computed on startup)
systemctl restart harmonylite

# Option B: Run cleanup command
harmonylite -cleanup -db mydb.db
```

**Note:** During the migration window, nodes with older schemas will pause replication and NATS will redeliver once schemas converge.

---

## References

- [Atlas - Database Schema as Code](https://atlasgo.io/) - Schema management tool used for introspection
- [Atlas SQLite Driver](https://github.com/ariga/atlas/tree/master/sql/sqlite) - SQLite-specific implementation
- [SQLite Schema Introspection](https://www.sqlite.org/pragma.html#pragma_table_info)
- [NATS KeyValue](https://docs.nats.io/nats-concepts/jetstream/key-value-store)
- [CockroachDB Schema Changes](https://www.cockroachlabs.com/docs/stable/online-schema-changes.html) (inspiration for coordination)
- [Vitess Schema Management](https://vitess.io/docs/user-guides/schema-changes/) (inspiration for policies)
