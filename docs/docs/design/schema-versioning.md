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
3. **Enable graceful handling** of schema differences during rolling upgrades by queuing incompatible events

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
    tables_hash TEXT NOT NULL,              -- Hash of watched/replicated tables
    updated_at INTEGER NOT NULL,
    harmonylite_version TEXT
);
```

**Fields:**
- `tables_hash`: SHA-256 hash of replicated/watched table schemas (deterministic)
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

// ComputeTableHash computes a deterministic SHA-256 hash for a single table
func (sm *SchemaManager) ComputeTableHash(ctx context.Context, tableName string) (string, error) {
    inspected, err := sm.InspectTables(ctx, []string{tableName})
    if err != nil {
        return "", err
    }
    if len(inspected) == 0 {
        return "", fmt.Errorf("table %q not found", tableName)
    }

    h := sha256.New()
    if err := hashTable(h, inspected[0]); err != nil {
        return "", err
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
    TableHash string `cbor:"th,omitempty"`  // Hash of table schema at creation
}
```

**Backward Compatibility:**
- Use CBOR `omitempty` tags
- Old events without `TableHash` are treated as "unknown schema"

### 4. Schema Hash Caching and Validation

To avoid expensive per-event validation, table hashes are computed once and cached:

```go
// db/schema_cache.go

// SchemaCache holds precomputed table hashes for fast validation
type SchemaCache struct {
    mu          sync.RWMutex
    tableHashes map[string]string  // table name -> hash
    tablesHash  string             // combined hash of all watched tables
}

// Initialize computes and caches hashes for all watched tables
func (sc *SchemaCache) Initialize(ctx context.Context, sm *SchemaManager, tables []string) error {
    sc.mu.Lock()
    defer sc.mu.Unlock()

    sc.tableHashes = make(map[string]string)
    for _, table := range tables {
        hash, err := sm.ComputeTableHash(ctx, table)
        if err != nil {
            return fmt.Errorf("computing hash for %s: %w", table, err)
        }
        sc.tableHashes[table] = hash
    }

    // Compute combined hash
    sc.tablesHash, _ = sm.ComputeSchemaHash(ctx, tables)
    return nil
}

// GetTableHash returns the cached hash for a table (O(1) map lookup)
func (sc *SchemaCache) GetTableHash(table string) (string, bool) {
    sc.mu.RLock()
    defer sc.mu.RUnlock()
    hash, ok := sc.tableHashes[table]
    return hash, ok
}

// Invalidate clears the cache, forcing recomputation on next access
func (sc *SchemaCache) Invalidate() {
    sc.mu.Lock()
    defer sc.mu.Unlock()
    sc.tableHashes = nil
    sc.tablesHash = ""
}
```

**Per-Event Validation:**

The validation in the replication hot path is a simple string comparison:

```go
// logstream/replicator.go

func (r *Replicator) validateAndApplyEvent(event *ChangeLogEvent) error {
    // Fast path: hash comparison only (O(1))
    if event.TableHash != "" {
        localHash, ok := r.schemaCache.GetTableHash(event.TableName)
        if !ok {
            // Table not in watched list - this shouldn't happen
            return fmt.Errorf("unknown table: %s", event.TableName)
        }
        if event.TableHash != localHash {
            // Schema mismatch - queue for later
            log.Warn().
                Str("table", event.TableName).
                Str("event_hash", event.TableHash[:8]).
                Str("local_hash", localHash[:8]).
                Msg("Schema mismatch, queuing event")
            return r.db.QueuePendingEvent(event)
        }
    }

    // Hashes match (or no hash in event) - apply directly
    return r.db.ReplicateRow(event)
}
```

**Performance Characteristics:**

| Operation | Cost | When |
|-----------|------|------|
| Hash computation | O(columns) + PRAGMA calls | Once at startup, on `-cleanup`, or schema change detection |
| Per-event validation | O(1) string comparison | Every incoming event |
| Cache lookup | O(1) map access with RLock | Every incoming event |

**Cache Invalidation:**

The cache is invalidated and recomputed:
- On HarmonyLite startup
- When running `harmonylite -cleanup`
- When a schema change is detected (future: via SQLite update hook on `sqlite_master`)

### 5. Schema Mismatch Handling (Queue Policy)

When a schema mismatch is detected (hash comparison fails), events are queued for later replay:

```toml
# config.toml

[schema]
# Maximum time to keep pending events before discarding
pending_event_ttl = "168h"  # 7 days
```

**Behavior:** When an incoming event's `TableHash` doesn't match the cached local hash, the event is stored in a pending queue. Pending events are automatically replayed on node restart (see Section 7).

### 6. Schema Registry via NATS KV

Broadcast schema state across the cluster using NATS KeyValue:

```go
// logstream/schema_registry.go

const SchemaRegistryBucket = "harmonylite-schema-registry"

type NodeSchemaState struct {
    NodeId             uint64            `json:"node_id"`
    TablesHash         string            `json:"tables_hash"`         // Combined hash of all watched tables
    Tables             map[string]string `json:"tables"`              // table -> hash
    HarmonyLiteVersion string            `json:"harmonylite_version"`
    UpdatedAt          time.Time         `json:"updated_at"`
}

func (r *Replicator) PublishSchemaState() error {
    state := NodeSchemaState{
        NodeId:             r.nodeId,
        TablesHash:         r.db.GetTablesHash(),
        Tables:             r.db.GetAllTableHashes(),
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
            referenceHash = state.TablesHash
        } else if state.TablesHash != referenceHash {
            report.Consistent = false
            report.Mismatches = append(report.Mismatches, SchemaMismatch{
                NodeId:       nodeId,
                ExpectedHash: referenceHash,
                ActualHash:   state.TablesHash,
            })
        }
    }

    return report, nil
}
```

### 7. Automatic Pending Event Replay

Pending events are automatically replayed on node startup, before normal replication begins. This ensures proper event ordering without requiring manual intervention.

#### Pending Events Table

```sql
CREATE TABLE IF NOT EXISTS __harmonylite__pending_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_data BLOB NOT NULL,
    table_name TEXT NOT NULL,
    required_table_hash TEXT,
    queued_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pending_table ON __harmonylite__pending_events(table_name);
```

#### Dead-Letter Table

Events that fail to apply (schema matches but apply fails, e.g., constraint violation) are moved to a dead-letter table for manual inspection:

```sql
CREATE TABLE IF NOT EXISTS __harmonylite__dead_letter_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    original_event_id INTEGER,          -- ID from pending_events table
    event_data BLOB NOT NULL,
    table_name TEXT NOT NULL,
    table_hash TEXT,
    error_message TEXT NOT NULL,        -- Why it failed
    queued_at INTEGER NOT NULL,         -- When originally queued
    failed_at INTEGER NOT NULL,         -- When moved to dead-letter
    source_node_id INTEGER              -- Node that originated the event
);
CREATE INDEX IF NOT EXISTS idx_dead_letter_table ON __harmonylite__dead_letter_events(table_name);
```

Users can query dead-letter events directly:
```bash
sqlite3 mydb.db "SELECT table_name, error_message, datetime(failed_at, 'unixepoch') FROM __harmonylite__dead_letter_events"
```

#### Startup Sequence

```go
// Startup sequence in main.go / replicator initialization

func (r *Replicator) Start() error {
    // 1. Initialize schema cache
    if err := r.schemaCache.Initialize(ctx, r.schemaManager, r.watchedTables); err != nil {
        return err
    }

    // 2. Replay pending events BEFORE starting replication
    replayed, warned, failed := r.replayPendingEvents()
    if replayed > 0 {
        log.Info().Int("count", replayed).Msg("Replayed pending events")
    }
    if warned > 0 {
        log.Warn().Int("count", warned).Msg("Pending events still incompatible with current schema")
    }
    if failed > 0 {
        log.Warn().Int("count", failed).Msg("Events moved to dead-letter table")
    }

    // 3. Start normal replication from NATS
    return r.startReplication()
}
```

#### Replay Logic

```go
// db/pending_events.go

func (db *SqliteStreamDB) QueuePendingEvent(event *ChangeLogEvent) error {
    data, _ := cbor.Marshal(event)
    _, err := db.db.Exec(`
        INSERT INTO __harmonylite__pending_events
        (event_data, table_name, required_table_hash, queued_at)
        VALUES (?, ?, ?, ?)`,
        data, event.TableName, event.TableHash, time.Now().Unix())
    return err
}

func (db *SqliteStreamDB) ReplayPendingEvents(schemaCache *SchemaCache) (replayed, stillPending, deadLettered int) {
    rows, err := db.db.Query(`
        SELECT id, event_data, table_name, required_table_hash, queued_at
        FROM __harmonylite__pending_events
        ORDER BY id ASC`)
    if err != nil {
        return 0, 0, 0
    }
    defer rows.Close()

    for rows.Next() {
        var id, queuedAt int64
        var data []byte
        var tableName, requiredHash string
        rows.Scan(&id, &data, &tableName, &requiredHash, &queuedAt)

        var event ChangeLogEvent
        cbor.Unmarshal(data, &event)

        // Check if schema now matches
        localHash, _ := schemaCache.GetTableHash(tableName)
        if requiredHash != "" && requiredHash != localHash {
            // Still incompatible - keep in queue
            stillPending++
            continue
        }

        // Schema matches - try to apply
        err := db.replicateRow(&event)
        if err != nil {
            // Apply failed - move to dead-letter
            db.moveToDeadLetter(id, &event, data, queuedAt, err.Error())
            deadLettered++
        } else {
            // Success - remove from pending
            db.db.Exec("DELETE FROM __harmonylite__pending_events WHERE id = ?", id)
            replayed++
        }
    }

    return replayed, stillPending, deadLettered
}

func (db *SqliteStreamDB) moveToDeadLetter(originalId int64, event *ChangeLogEvent, data []byte, queuedAt int64, errMsg string) {
    db.db.Exec(`
        INSERT INTO __harmonylite__dead_letter_events
        (original_event_id, event_data, table_name, table_hash, error_message, queued_at, failed_at, source_node_id)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
        originalId, data, event.TableName, event.TableHash, errMsg, queuedAt, time.Now().Unix(), event.NodeId)
    
    db.db.Exec("DELETE FROM __harmonylite__pending_events WHERE id = ?", originalId)
}
```

**Three outcomes per event:**
1. **Schema still mismatched** → stays in pending queue, logged as warning
2. **Schema matches, apply succeeds** → removed from pending
3. **Schema matches, apply fails** → moved to dead-letter table

#### Startup Logging

```
INFO  Starting HarmonyLite v1.2.0
INFO  Initializing schema cache for 3 tables
INFO  Replaying pending events...
INFO  Replayed pending events                    count=42
WARN  Pending events still incompatible          count=5 tables=["orders"]
WARN  Events moved to dead-letter table          count=2 tables=["users"]
INFO  Starting NATS replication...
```
```

### 8. CLI Commands

Add schema management commands:

```bash
# Check schema status (local node)
harmonylite schema status
# Output:
# Local Schema Status
# ===================
# Tables Hash: f6e5d4c3b2a1...
# Updated At: 2025-01-17 10:30:00
# HarmonyLite Version: 1.2.0
#
# Watched Tables:
#   users: hash=abc123...
#   orders: hash=def456...

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

# Check pending and dead-letter events
harmonylite schema pending
# Output:
# Pending Events by Table
# =======================
# users: 5 events (oldest: 2025-01-17 10:30:00)
# orders: 0 events
#
# Dead-Letter Events by Table
# ===========================
# users: 2 events (use sqlite3 to inspect __harmonylite__dead_letter_events)
```

**Note:** There is no manual `schema replay` command. Pending events are automatically replayed on node restart.

---

## Implementation Phases

### Phase 1: Foundation (Schema Tracking with Atlas)
- [ ] Add `ariga.io/atlas` dependency to `go.mod`
- [ ] Create `db/schema_manager.go` with `SchemaManager` type wrapping Atlas SQLite driver
- [ ] Implement `InspectTables()`, `ComputeSchemaHash()`, `ComputeTableHash()`
- [ ] Create `db/schema_cache.go` with `SchemaCache` type for caching table hashes
- [ ] Add `__harmonylite__schema_version` table creation in `InstallCDC()`
- [ ] Initialize schema cache on startup
- [ ] Update schema state on `-cleanup` command (invalidate and recompute cache)
- [ ] Add `harmonylite schema status` CLI command (local only)

### Phase 2: Event Enhancement
- [ ] Add `TableHash` field to `ChangeLogEvent`
- [ ] Populate field during event creation using cached hash (O(1) lookup)
- [ ] Ensure backward compatibility with old events (CBOR `omitempty`)

### Phase 3: Validation and Automatic Replay
- [ ] Add hash comparison in replication hot path (O(1) string comparison)
- [ ] Create `__harmonylite__pending_events` table
- [ ] Create `__harmonylite__dead_letter_events` table
- [ ] Implement `QueuePendingEvent()` in `db/pending_events.go`
- [ ] Implement `ReplayPendingEvents()` with automatic startup replay
- [ ] Implement `moveToDeadLetter()` for failed event handling
- [ ] Add pending event TTL and cleanup
- [ ] Add `harmonylite schema pending` command (shows pending and dead-letter counts)
- [ ] Add schema mismatch metrics (Prometheus counters)
- [ ] Add pending events gauge metric
- [ ] Add dead-letter events gauge metric

### Phase 4: Cluster Visibility
- [ ] Create NATS KV bucket `harmonylite-schema-registry`
- [ ] Create `logstream/schema_registry.go` with registry client
- [ ] Implement `PublishSchemaState()` on startup and schema change
- [ ] Implement `GetClusterSchemaState()` and `CheckClusterSchemaConsistency()`
- [ ] Update `harmonylite schema status --cluster` to show cluster view

---

## Configuration Reference

```toml
[schema]
# Maximum time to keep pending events before discarding
pending_event_ttl = "168h"  # 7 days
```

---

## Metrics and Observability

### Prometheus Metrics

```
# Tables hash on this node (for alerting on changes)
harmonylite_schema_tables_hash_info{node_id="1", hash="a1b2c3d4"} 1

# Number of nodes with matching schema
harmonylite_cluster_schema_consistent_nodes 2

# Total nodes in cluster
harmonylite_cluster_nodes_total 3

# Events queued due to schema mismatch
harmonylite_schema_mismatch_events_total{table="users"} 42

# Pending events waiting for schema match
harmonylite_pending_events{table="users"} 156

# Dead-letter events (failed to apply)
harmonylite_dead_letter_events{table="users"} 2

# Time since oldest pending event (seconds)
harmonylite_pending_events_age_seconds{table="users"} 3600
```

### Health Check Extension

Extend existing health check endpoint:

```json
{
  "status": "degraded",
  "checks": {
    "schema": {
      "status": "warning",
      "tables_hash": "a1b2c3d4e5f6",
      "cluster_consistent": false,
      "mismatched_nodes": [3],
      "pending_events": {
        "users": 42
      }
    }
  }
}
```

---

## Migration Guide

### Performing Schema Migrations

Schema migrations are performed manually on each node. Incompatible events are automatically queued and replayed on restart.

```bash
# 1. Apply DDL on Node 1
sqlite3 mydb.db "ALTER TABLE users ADD COLUMN email TEXT"

# 2. Restart HarmonyLite on Node 1 (pending events are automatically replayed)
systemctl restart harmonylite
# Or run -cleanup to update schema state without full restart:
harmonylite -cleanup -db mydb.db

# 3. Repeat for other nodes
# During migration window, events from migrated nodes are queued on non-migrated nodes

# 4. After all nodes are migrated and restarted, pending events are automatically replayed
# Check for any remaining issues:
harmonylite schema pending
```

**Note:** During the migration window, nodes with older schemas will queue events from nodes with newer schemas. Once a node has the matching schema and restarts, pending events are automatically replayed. Events that fail to apply (e.g., constraint violations) are moved to the dead-letter table for manual inspection.

---

## Open Questions

1. **Event Retention**: How long to keep pending events before discarding? (Current default: 7 days)

2. **Hash Stability**: If Atlas changes its type normalization in a future version, hashes could change. Should we pin to a specific hash algorithm version?

---

## References

- [Atlas - Database Schema as Code](https://atlasgo.io/) - Schema management tool used for introspection
- [Atlas SQLite Driver](https://github.com/ariga/atlas/tree/master/sql/sqlite) - SQLite-specific implementation
- [SQLite Schema Introspection](https://www.sqlite.org/pragma.html#pragma_table_info)
- [NATS KeyValue](https://docs.nats.io/nats-concepts/jetstream/key-value-store)
- [CockroachDB Schema Changes](https://www.cockroachlabs.com/docs/stable/online-schema-changes.html) (inspiration for coordination)
- [Vitess Schema Management](https://vitess.io/docs/user-guides/schema-changes/) (inspiration for policies)
