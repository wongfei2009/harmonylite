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
- Multiple mismatch policies (only queue/pending is supported)

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

### 1. Schema Version Tracking

Add a schema version table to each node:

```sql
CREATE TABLE IF NOT EXISTS __harmonylite__schema_version (
    id INTEGER PRIMARY KEY CHECK (id = 1),  -- Single row constraint
    version INTEGER NOT NULL DEFAULT 0,
    schema_hash TEXT NOT NULL,
    tables_hash TEXT NOT NULL,              -- Hash of watched tables only
    migrated_at INTEGER NOT NULL,
    harmonylite_version TEXT,
    migration_name TEXT                     -- Optional: descriptive name
);
```

**Fields:**
- `version`: Monotonically increasing integer, incremented on each schema change
- `schema_hash`: SHA-256 hash of all table schemas (deterministic)
- `tables_hash`: SHA-256 hash of only replicated/watched tables
- `migrated_at`: Unix timestamp of last migration
- `harmonylite_version`: HarmonyLite binary version that applied the migration
- `migration_name`: Human-readable migration identifier (e.g., "add_user_email_column")

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

    // New fields for schema versioning
    SchemaVersion int64  `cbor:"sv,omitempty"`  // Schema version when event created
    TableHash     string `cbor:"th,omitempty"`  // Hash of table schema at creation
}
```

**Backward Compatibility:**
- Use CBOR `omitempty` tags
- Old events without these fields are treated as "unknown schema version"
- Nodes can be configured to accept or reject events without version info

### 4. Pre-Replication Validation

Validate schema compatibility before applying events:

```go
// db/schema_validator.go

type SchemaValidationResult struct {
    Compatible    bool
    Errors        []SchemaError
    Warnings      []SchemaWarning
}

type SchemaError struct {
    Type    string  // "missing_column", "missing_table", "type_mismatch"
    Table   string
    Column  string
    Details string
}

func (db *SqliteStreamDB) ValidateEventSchema(event *ChangeLogEvent) SchemaValidationResult {
    result := SchemaValidationResult{Compatible: true}

    localSchema := db.watchTablesSchema[event.TableName]

    // Check 1: Table exists
    if localSchema == nil {
        result.Compatible = false
        result.Errors = append(result.Errors, SchemaError{
            Type:    "missing_table",
            Table:   event.TableName,
            Details: "Table does not exist on this node",
        })
        return result
    }

    // Check 2: All event columns exist locally
    localCols := make(map[string]*ColumnInfo)
    for _, col := range localSchema {
        localCols[col.Name] = col
    }

    for colName := range event.Row {
        if _, exists := localCols[colName]; !exists {
            result.Compatible = false
            result.Errors = append(result.Errors, SchemaError{
                Type:    "missing_column",
                Table:   event.TableName,
                Column:  colName,
                Details: "Column exists in event but not on this node",
            })
        }
    }

    // Check 3: Table hash comparison (if available)
    if event.TableHash != "" {
        localHash, _ := db.ComputeTableHash(event.TableName)
        if event.TableHash != localHash {
            result.Warnings = append(result.Warnings, SchemaWarning{
                Type:    "schema_drift",
                Table:   event.TableName,
                Details: fmt.Sprintf("Table hash mismatch: event=%s local=%s",
                    event.TableHash[:8], localHash[:8]),
            })
        }
    }

    return result
}
```

### 5. Schema Mismatch Handling (Queue Policy)

When a schema mismatch is detected, events are queued for later replay:

```toml
# config.toml

[schema]
# Include schema version in events (increases event size slightly)
include_version_in_events = true

# Reject events without schema version (for strict environments)
require_version_in_events = false

# Log level for schema warnings
warning_log_level = "warn"

# Maximum time to wait for pending events before discarding
pending_event_ttl = "168h"  # 7 days

# Maximum pending events per table before alerting
pending_event_alert_threshold = 1000
```

**Behavior:** When an incoming event cannot be applied due to schema mismatch (e.g., missing column), the event is stored in a pending queue. After the user applies the DDL migration manually, they run `harmonylite schema replay` to apply the queued events.

```go
// logstream/replicator.go

func (r *Replicator) handleSchemaMismatch(event *ChangeLogEvent, result SchemaValidationResult) error {
    log.Warn().
        Str("table", event.TableName).
        Interface("errors", result.Errors).
        Msg("Schema mismatch detected, queuing event for later replay")
    
    return r.db.QueuePendingEvent(event)
}
```

### 6. Schema Registry via NATS KV

Broadcast schema state across the cluster using NATS KeyValue:

```go
// logstream/schema_registry.go

const SchemaRegistryBucket = "harmonylite-schema-registry"

type NodeSchemaState struct {
    NodeId            uint64            `json:"node_id"`
    SchemaVersion     int64             `json:"schema_version"`
    SchemaHash        string            `json:"schema_hash"`
    TablesHash        string            `json:"tables_hash"`
    Tables            map[string]string `json:"tables"`  // table -> hash
    HarmonyLiteVersion string           `json:"harmonylite_version"`
    UpdatedAt         time.Time         `json:"updated_at"`
}

func (r *Replicator) PublishSchemaState() error {
    state := NodeSchemaState{
        NodeId:            r.nodeId,
        SchemaVersion:     r.db.GetSchemaVersion(),
        SchemaHash:        r.db.GetSchemaHash(),
        Tables:            r.db.GetAllTableHashes(),
        HarmonyLiteVersion: version.Version,
        UpdatedAt:         time.Now(),
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

### 7. Queued Event Replay

For `queue` policy, implement a pending event store:

```go
// db/pending_events.go

// SQLite table for pending events
const PendingEventsSchema = `
CREATE TABLE IF NOT EXISTS __harmonylite__pending_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_data BLOB NOT NULL,
    table_name TEXT NOT NULL,
    required_schema_version INTEGER,
    required_table_hash TEXT,
    queued_at INTEGER NOT NULL,
    retry_count INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_pending_table ON __harmonylite__pending_events(table_name);
`

func (db *SqliteStreamDB) QueuePendingEvent(event *ChangeLogEvent) error {
    data, _ := cbor.Marshal(event)
    _, err := db.db.Exec(`
        INSERT INTO __harmonylite__pending_events
        (event_data, table_name, required_schema_version, required_table_hash, queued_at)
        VALUES (?, ?, ?, ?, ?)`,
        data, event.TableName, event.SchemaVersion, event.TableHash, time.Now().Unix())
    return err
}

func (db *SqliteStreamDB) ReplayPendingEvents(tableName string) (int, error) {
    // Called after schema migration completes
    rows, err := db.db.Query(`
        SELECT id, event_data FROM __harmonylite__pending_events
        WHERE table_name = ? ORDER BY id ASC`, tableName)
    if err != nil {
        return 0, err
    }
    defer rows.Close()

    replayed := 0
    for rows.Next() {
        var id int64
        var data []byte
        rows.Scan(&id, &data)

        var event ChangeLogEvent
        cbor.Unmarshal(data, &event)

        // Re-validate and apply
        result := db.ValidateEventSchema(&event)
        if result.Compatible {
            db.replicateRow(&event)
            db.db.Exec("DELETE FROM __harmonylite__pending_events WHERE id = ?", id)
            replayed++
        }
    }

    return replayed, nil
}
```

### 8. CLI Commands

Add schema management commands:

```bash
# Check schema status (local node)
harmonylite schema status
# Output:
# Local Schema Status
# ===================
# Schema Version: 5
# Schema Hash: a1b2c3d4e5f6...
# Tables Hash: f6e5d4c3b2a1...
# Migrated At: 2025-01-17 10:30:00
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
# Node 1: v5 (hash: a1b2c3d4) - CURRENT
# Node 2: v5 (hash: a1b2c3d4) - MATCH
# Node 3: v4 (hash: e5f6g7h8) - MISMATCH

# Check pending events
harmonylite schema pending
# Output:
# Pending Events by Table
# =======================
# users: 42 events (oldest: 2025-01-17 10:30:00)
# orders: 0 events

# Replay pending events after manual DDL migration
harmonylite schema replay --table users
# Output:
# Replaying pending events for table 'users'...
# Replayed: 42 events
# Failed: 0 events
# Remaining: 0 events
```

---

## Implementation Phases

### Phase 1: Foundation (Schema Tracking with Atlas)
- [ ] Add `ariga.io/atlas` dependency to `go.mod`
- [ ] Create `db/schema_manager.go` with `SchemaManager` type wrapping Atlas SQLite driver
- [ ] Implement `InspectTables()`, `ComputeSchemaHash()`, `ComputeTableHash()`
- [ ] Add `__harmonylite__schema_version` table creation in `InstallCDC()`
- [ ] Add `GetSchemaVersion()` and `UpdateSchemaVersion()` methods to `SqliteStreamDB`
- [ ] Update schema version on `-cleanup` command
- [ ] Add `harmonylite schema status` CLI command (local only)

### Phase 2: Event Enhancement
- [ ] Add `SchemaVersion` and `TableHash` fields to `ChangeLogEvent`
- [ ] Populate fields during event creation in `consumeChangeLogs()`
- [ ] Ensure backward compatibility with old events (CBOR `omitempty`)
- [ ] Add schema version caching to avoid repeated computation

### Phase 3: Validation and Queue
- [ ] Create `db/schema_validator.go` with `SchemaValidator` type
- [ ] Implement `ValidateEventSchema()` using cached schema info
- [ ] Create `__harmonylite__pending_events` table
- [ ] Implement `QueuePendingEvent()` in `db/pending_events.go`
- [ ] Implement `ReplayPendingEvents()` with validation
- [ ] Add pending event TTL and cleanup
- [ ] Add `harmonylite schema pending` and `harmonylite schema replay` commands
- [ ] Add schema mismatch metrics (Prometheus counters)
- [ ] Add pending events gauge metric

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
# Include schema version metadata in replication events
# Increases event size by ~50 bytes but enables version tracking
include_version_in_events = true

# Reject events that don't have schema version metadata
# Enable this after all nodes are upgraded to version with schema support
require_version_in_events = false

# How often to publish schema state to cluster registry (0 = on change only)
registry_publish_interval = "0"

# Maximum time to wait for pending events before discarding
pending_event_ttl = "168h"  # 7 days

# Maximum pending events per table before alerting
pending_event_alert_threshold = 1000
```

---

## Metrics and Observability

### Prometheus Metrics

```
# Schema version on this node
harmonylite_schema_version{node_id="1"} 5

# Number of nodes with matching schema
harmonylite_cluster_schema_consistent_nodes 2

# Total nodes in cluster
harmonylite_cluster_nodes_total 3

# Events queued due to schema mismatch
harmonylite_schema_mismatch_events_total{table="users"} 42

# Pending events waiting for schema match
harmonylite_pending_events{table="users"} 156

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
      "schema_version": 5,
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

### Upgrading Existing Clusters

1. **Upgrade HarmonyLite binary** on all nodes (supports old event format)
2. **Enable schema tracking** with `include_version_in_events = false` initially
3. **Verify all nodes upgraded** via `harmonylite schema status`
4. **Enable version in events**: Set `include_version_in_events = true`
5. **Optionally enable strict mode**: Set `require_version_in_events = true`

### Performing Schema Migrations

Schema migrations are performed manually on each node. Incompatible events are automatically queued.

```bash
# 1. Apply DDL on Node 1
sqlite3 mydb.db "ALTER TABLE users ADD COLUMN email TEXT"

# 2. Restart HarmonyLite on Node 1 (or run -cleanup to update schema version)
harmonylite -cleanup -db mydb.db

# 3. Repeat for other nodes
# During migration window, events from migrated nodes are queued on non-migrated nodes

# 4. After all nodes are migrated, replay pending events
harmonylite schema replay --table users
```

**Note:** During the migration window, nodes with older schemas will queue events from nodes with newer schemas. Once all nodes have the same schema, run `harmonylite schema replay` to apply queued events.

---

## Open Questions

1. **Version Numbering**: Global monotonic version vs. per-table versions? (Current design uses global version)

2. **Event Retention**: How long to keep pending events before discarding? (Current default: 7 days)

3. **Automatic Replay**: Should pending events be automatically replayed when schema matches, or require manual `harmonylite schema replay`?

4. **Hash Stability**: If Atlas changes its type normalization in a future version, hashes could change. Should we pin to a specific hash algorithm version?

---

## References

- [Atlas - Database Schema as Code](https://atlasgo.io/) - Schema management tool used for introspection
- [Atlas SQLite Driver](https://github.com/ariga/atlas/tree/master/sql/sqlite) - SQLite-specific implementation
- [SQLite Schema Introspection](https://www.sqlite.org/pragma.html#pragma_table_info)
- [NATS KeyValue](https://docs.nats.io/nats-concepts/jetstream/key-value-store)
- [CockroachDB Schema Changes](https://www.cockroachlabs.com/docs/stable/online-schema-changes.html) (inspiration for coordination)
- [Vitess Schema Management](https://vitess.io/docs/user-guides/schema-changes/) (inspiration for policies)
