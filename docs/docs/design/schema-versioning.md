# Schema Versioning and Migration Design

**Status:** Draft
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
    mu         sync.RWMutex
    schemaHash string
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
    return nil
}

// GetSchemaHash returns the cached schema hash (O(1))
func (sc *SchemaCache) GetSchemaHash() string {
    sc.mu.RLock()
    defer sc.mu.RUnlock()
    return sc.schemaHash
}

// Invalidate clears the cache, forcing recomputation on next access
func (sc *SchemaCache) Invalidate() {
    sc.mu.Lock()
    defer sc.mu.Unlock()
    sc.schemaHash = ""
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
        if event.SchemaHash != localHash {
            log.Warn().
                Str("event_hash", event.SchemaHash[:8]).
                Str("local_hash", localHash[:8]).
                Msg("Schema mismatch, pausing replication")
            msg.NakWithDelay(30 * time.Second)
            return nil
        }
    }

    // Hashes match (or no hash in event) - apply directly
    return r.db.ReplicateRow(event)
}
```

**Performance Characteristics:**

| Operation | Cost | When |
|-----------|------|------|
| Hash computation | O(tables + columns) + PRAGMA calls | Once at startup, on `-cleanup`, or schema change detection |
| Per-event validation | O(1) string comparison | Every incoming event |
| Cache lookup | O(1) string read with RLock | Every incoming event |

**Cache Invalidation:**

The cache is invalidated and recomputed:
- On HarmonyLite startup
- When running `harmonylite -cleanup`
- When a schema change is detected (future: via SQLite update hook on `sqlite_master`)

### 5. Schema Mismatch Handling (Pause Policy)

When a schema mismatch is detected (hash comparison fails), replication pauses for that shard by NAK-ing the message with a delay. The sequence map is not advanced, so ordering is preserved. Once the local schema is updated, NATS redelivers from the same sequence and replication resumes.

**Behavior:** When an incoming event's `SchemaHash` doesn't match the cached local hash, the consumer logs a warning, NAKs with a delay, and does not apply the event.

### 6. Schema Registry via NATS KV

Broadcast schema state across the cluster using NATS KeyValue:

```go
// logstream/schema_registry.go

const SchemaRegistryBucket = "harmonylite-schema-registry"

type NodeSchemaState struct {
    NodeId             uint64            `json:"node_id"`
    SchemaHash         string            `json:"schema_hash"`         // Combined hash of all watched tables
    HarmonyLiteVersion string            `json:"harmonylite_version"`
    UpdatedAt          time.Time         `json:"updated_at"`
}

func (r *Replicator) PublishSchemaState() error {
    state := NodeSchemaState{
        NodeId:             r.nodeId,
        SchemaHash:         r.db.GetSchemaHash(),
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

### 7. Apply Failure Handling (Dead-Letter)

Events that match the schema hash but fail to apply (e.g., constraint violations) can be moved to a dead-letter table for manual inspection.

```sql
CREATE TABLE IF NOT EXISTS __harmonylite__dead_letter_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_data BLOB NOT NULL,
    table_name TEXT NOT NULL,
    schema_hash TEXT,
    error_message TEXT NOT NULL,
    failed_at INTEGER NOT NULL,
    source_node_id INTEGER,
    stream_name TEXT NOT NULL,
    stream_seq INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dead_letter_table ON __harmonylite__dead_letter_events(table_name);
```

Users can query dead-letter events directly:
```bash
sqlite3 mydb.db "SELECT table_name, error_message, datetime(failed_at, 'unixepoch') FROM __harmonylite__dead_letter_events"
```

### 8. CLI Commands

Add schema management commands:

```bash
# Check schema status (local node)
harmonylite schema status
# Output:
# Local Schema Status
# ===================
# Schema Hash: f6e5d4c3b2a1...
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
# Consistent: No
#
# Hash Groups:
#   a1b2c3d4: Node 1, Node 2
#   e5f6g7h8: Node 3

```

---

## Implementation Phases

### Phase 1: Foundation (Schema Tracking with Atlas)
- [ ] Add `ariga.io/atlas` dependency to `go.mod`
- [ ] Create `db/schema_manager.go` with `SchemaManager` type wrapping Atlas SQLite driver
- [ ] Implement `InspectTables()` and `ComputeSchemaHash()`
- [ ] Create `db/schema_cache.go` with `SchemaCache` type for caching the schema hash
- [ ] Add `__harmonylite__schema_version` table creation in `InstallCDC()`
- [ ] Initialize schema cache on startup
- [ ] Update schema state on `-cleanup` command (invalidate and recompute cache)
- [ ] Add `harmonylite schema status` CLI command (local only)

### Phase 2: Event Enhancement
- [ ] Add `SchemaHash` field to `ChangeLogEvent`
- [ ] Populate field during event creation using cached hash (O(1) lookup)
- [ ] Ensure backward compatibility with old events (CBOR `omitempty`)

### Phase 3: Validation and Pause-on-Mismatch
- [ ] Add hash comparison in replication hot path (O(1) string comparison)
- [ ] NAK with delay when schema hash mismatches (pause replication)
- [ ] Create `__harmonylite__dead_letter_events` table
- [ ] Implement dead-letter capture for apply failures
- [ ] Add schema mismatch metrics (Prometheus counters)
- [ ] Add dead-letter events gauge metric

### Phase 4: Cluster Visibility
- [ ] Create NATS KV bucket `harmonylite-schema-registry`
- [ ] Create `logstream/schema_registry.go` with registry client
- [ ] Implement `PublishSchemaState()` on startup and schema change
- [ ] Implement `GetClusterSchemaState()` and `CheckClusterSchemaConsistency()`
- [ ] Update `harmonylite schema status --cluster` to show cluster view

---

## Configuration Reference

No additional configuration required in the initial version.

---

## Metrics and Observability

### Prometheus Metrics

```
# Schema hash on this node (for alerting on changes)
harmonylite_schema_hash_info{node_id="1", hash="a1b2c3d4"} 1

# Number of nodes with matching schema
harmonylite_cluster_schema_consistent_nodes 2

# Total nodes in cluster
harmonylite_cluster_nodes_total 3

# Events paused due to schema mismatch
harmonylite_schema_mismatch_events_total 42

# Dead-letter events (failed to apply)
harmonylite_dead_letter_events_total{table="users"} 2
```

### Health Check Extension

Extend existing health check endpoint:

```json
{
  "status": "degraded",
  "checks": {
    "schema": {
      "status": "warning",
      "schema_hash": "a1b2c3d4e5f6",
      "cluster_consistent": false,
      "mismatched_nodes": [3]
    }
  }
}
```

---

## Migration Guide

### Performing Schema Migrations

Schema migrations are performed manually on each node. Incompatible events pause replication until schemas converge.

```bash
# 1. Apply DDL on Node 1
sqlite3 mydb.db "ALTER TABLE users ADD COLUMN email TEXT"

# 2. Restart HarmonyLite on Node 1 (schema hash is recalculated)
systemctl restart harmonylite
# Or run -cleanup to update schema state without full restart:
harmonylite -cleanup -db mydb.db

# 3. Repeat for other nodes
# During migration window, nodes with older schemas will pause replication

# 4. After all nodes are migrated and restarted, replication resumes automatically
```

**Note:** During the migration window, nodes with older schemas will pause replication and NATS will redeliver once schemas converge. Events that fail to apply (e.g., constraint violations) can be moved to the dead-letter table for manual inspection.

---

## Open Questions

1. **Stream Retention**: How long can replication stay paused before JetStream truncation forces a snapshot restore?

2. **Hash Stability**: If Atlas changes its type normalization in a future version, hashes could change. Should we pin to a specific hash algorithm version?

---

## References

- [Atlas - Database Schema as Code](https://atlasgo.io/) - Schema management tool used for introspection
- [Atlas SQLite Driver](https://github.com/ariga/atlas/tree/master/sql/sqlite) - SQLite-specific implementation
- [SQLite Schema Introspection](https://www.sqlite.org/pragma.html#pragma_table_info)
- [NATS KeyValue](https://docs.nats.io/nats-concepts/jetstream/key-value-store)
- [CockroachDB Schema Changes](https://www.cockroachlabs.com/docs/stable/online-schema-changes.html) (inspiration for coordination)
- [Vitess Schema Management](https://vitess.io/docs/user-guides/schema-changes/) (inspiration for policies)
