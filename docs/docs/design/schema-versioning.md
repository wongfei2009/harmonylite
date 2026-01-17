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
3. **Enable graceful handling** of schema differences during rolling upgrades
4. **Support coordinated migrations** with minimal downtime

## Non-Goals

- Automatic schema migration (DDL replication)
- Distributed transactions
- Strong consistency guarantees

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

Implement deterministic schema hashing:

```go
// db/schema_hash.go

type TableSchema struct {
    Name    string
    Columns []ColumnSchema
}

type ColumnSchema struct {
    Name       string
    Type       string
    NotNull    bool
    DefaultVal string
    PrimaryKey bool
}

func (db *SqliteStreamDB) ComputeSchemaHash() (string, error) {
    tables := db.getWatchedTables()
    sort.Strings(tables)  // Deterministic ordering

    h := sha256.New()
    for _, tableName := range tables {
        schema, err := db.getTableSchema(tableName)
        if err != nil {
            return "", err
        }
        // Sort columns by name for determinism
        sort.Slice(schema.Columns, func(i, j int) bool {
            return schema.Columns[i].Name < schema.Columns[j].Name
        })

        // Hash: tableName|col1:type1:notnull:pk|col2:type2:notnull:pk|...
        h.Write([]byte(tableName))
        for _, col := range schema.Columns {
            h.Write([]byte(fmt.Sprintf("|%s:%s:%t:%t",
                col.Name, col.Type, col.NotNull, col.PrimaryKey)))
        }
        h.Write([]byte("\n"))
    }

    return hex.EncodeToString(h.Sum(nil)), nil
}

func (db *SqliteStreamDB) ComputeTableHash(tableName string) (string, error) {
    // Same as above but for single table
}
```

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

### 5. Schema Mismatch Policies

Configurable behavior when schema mismatch is detected:

```toml
# config.toml

[schema]
# Policy when schema mismatch is detected
# Options: "strict", "skip", "partial", "queue"
mismatch_policy = "queue"

# Include schema version in events (increases event size slightly)
include_version_in_events = true

# Reject events without schema version (for strict environments)
require_version_in_events = false

# Log level for schema warnings
warning_log_level = "warn"
```

**Policy Definitions:**

| Policy | Behavior | Use Case |
|--------|----------|----------|
| `strict` | Reject event, stop replication, emit alert | Production with strict consistency requirements |
| `skip` | Log warning, skip incompatible events, continue | Development/testing |
| `partial` | Apply only columns that exist on target | Rolling upgrades with additive-only changes |
| `queue` | Hold events in pending queue until schema matches | Rolling upgrades with coordinated migrations |

```go
// logstream/replicator.go

func (r *Replicator) handleSchemaMismatch(event *ChangeLogEvent, result SchemaValidationResult) error {
    switch r.config.Schema.MismatchPolicy {
    case "strict":
        r.emitAlert(AlertSchemaMismatch, event, result)
        return ErrSchemaMismatchStrict

    case "skip":
        log.Warn().
            Str("table", event.TableName).
            Interface("errors", result.Errors).
            Msg("Skipping event due to schema mismatch")
        return nil  // Acknowledge and skip

    case "partial":
        return r.applyPartialEvent(event, result)

    case "queue":
        return r.queueForLaterReplay(event)

    default:
        return ErrUnknownPolicy
    }
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

### 8. Schema Migration Coordination Protocol

For zero-downtime coordinated migrations:

```go
// logstream/migration_coordinator.go

type MigrationPhase string

const (
    PhaseAnnounce    MigrationPhase = "announce"
    PhasePrepare     MigrationPhase = "prepare"
    PhaseReady       MigrationPhase = "ready"
    PhaseApply       MigrationPhase = "apply"
    PhaseComplete    MigrationPhase = "complete"
    PhaseRollback    MigrationPhase = "rollback"
)

type MigrationEvent struct {
    MigrationId   string         `json:"migration_id"`
    Phase         MigrationPhase `json:"phase"`
    FromVersion   int64          `json:"from_version"`
    ToVersion     int64          `json:"to_version"`
    MigrationName string         `json:"migration_name"`
    InitiatorNode uint64         `json:"initiator_node"`
    Timestamp     time.Time      `json:"timestamp"`
}

type NodeMigrationAck struct {
    NodeId      uint64         `json:"node_id"`
    MigrationId string         `json:"migration_id"`
    Phase       MigrationPhase `json:"phase"`
    Success     bool           `json:"success"`
    Error       string         `json:"error,omitempty"`
}
```

**Coordination Flow:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Schema Migration Coordination                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. ANNOUNCE                                                            │
│     Initiator → All Nodes: "Migration v5→v6 pending"                   │
│                                                                         │
│  2. PREPARE                                                             │
│     All Nodes: Pause new writes, drain pending change logs             │
│     All Nodes → Initiator: "PREPARE_ACK"                               │
│                                                                         │
│  3. READY                                                               │
│     Initiator: Wait for all PREPARE_ACKs (with timeout)                │
│     Initiator → All Nodes: "READY - apply migration"                   │
│                                                                         │
│  4. APPLY                                                               │
│     Each Node: Apply DDL migration locally                             │
│     Each Node: Update schema version table                             │
│     Each Node → Initiator: "APPLY_ACK" or "APPLY_FAIL"                 │
│                                                                         │
│  5. COMPLETE (if all succeed)                                           │
│     Initiator → All Nodes: "COMPLETE - resume writes"                  │
│     All Nodes: Resume normal operation                                  │
│                                                                         │
│  5. ROLLBACK (if any fail)                                              │
│     Initiator → All Nodes: "ROLLBACK"                                  │
│     All Nodes: Restore from pre-migration state                        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 9. CLI Commands

Add schema management commands:

```bash
# Check schema consistency across cluster
harmonylite schema status
# Output:
# Cluster Schema Status
# =====================
# Total Nodes: 3
# Consistent: No
#
# Node 1: v5 (hash: a1b2c3d4) - CURRENT
# Node 2: v5 (hash: a1b2c3d4) - MATCH
# Node 3: v4 (hash: e5f6g7h8) - MISMATCH
#
# Tables with differences:
#   users: Node 3 missing column 'email'

# Show detailed schema diff between nodes
harmonylite schema diff --node1 1 --node2 3
# Output:
# Schema Diff: Node 1 vs Node 3
# =============================
# Table: users
#   + email TEXT (exists on Node 1, missing on Node 3)

# Initiate coordinated migration
harmonylite schema migrate \
    --name "add_user_email" \
    --version 6 \
    --ddl "ALTER TABLE users ADD COLUMN email TEXT" \
    --timeout 60s

# Check pending events
harmonylite schema pending
# Output:
# Pending Events by Table
# =======================
# users: 42 events (oldest: 2025-01-17 10:30:00)
# orders: 0 events

# Replay pending events after migration
harmonylite schema replay --table users
```

---

## Implementation Phases

### Phase 1: Foundation (Schema Tracking)
- [ ] Add `__harmonylite__schema_version` table
- [ ] Implement `ComputeSchemaHash()` and `ComputeTableHash()`
- [ ] Update schema version on `-cleanup` command
- [ ] Add `harmonylite schema status` CLI command (local only)

### Phase 2: Event Enhancement
- [ ] Add `SchemaVersion` and `TableHash` fields to `ChangeLogEvent`
- [ ] Populate fields during event creation
- [ ] Ensure backward compatibility with old events

### Phase 3: Validation Layer
- [ ] Implement `ValidateEventSchema()`
- [ ] Add `mismatch_policy` configuration
- [ ] Implement `strict` and `skip` policies
- [ ] Add schema mismatch metrics/logging

### Phase 4: Cluster Visibility
- [ ] Create NATS KV schema registry
- [ ] Implement `PublishSchemaState()` on startup and schema change
- [ ] Implement `CheckClusterSchemaConsistency()`
- [ ] Add `harmonylite schema status` with cluster view
- [ ] Add `harmonylite schema diff` command

### Phase 5: Queue Policy
- [ ] Create `__harmonylite__pending_events` table
- [ ] Implement `QueuePendingEvent()`
- [ ] Implement `ReplayPendingEvents()`
- [ ] Add `harmonylite schema pending` and `harmonylite schema replay` commands

### Phase 6: Coordinated Migrations
- [ ] Implement migration coordination protocol
- [ ] Add `harmonylite schema migrate` command
- [ ] Implement rollback capability
- [ ] Add migration history tracking

---

## Configuration Reference

```toml
[schema]
# Policy when schema mismatch is detected during replication
# "strict"  - Reject event, stop replication, emit alert
# "skip"    - Log warning, skip incompatible events, continue
# "partial" - Apply only columns that exist on target
# "queue"   - Hold events until schema matches
mismatch_policy = "queue"

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

# Schema hash (as numeric for comparison)
harmonylite_schema_hash{node_id="1"} 1234567890

# Number of nodes with matching schema
harmonylite_cluster_schema_consistent_nodes 2

# Total nodes in cluster
harmonylite_cluster_nodes_total 3

# Events skipped due to schema mismatch
harmonylite_schema_mismatch_events_total{table="users",policy="skip"} 42

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

**Option A: Rolling upgrade (additive changes only)**
```bash
# Use "partial" policy during migration window
# 1. Add column with default on Node 1
# 2. Run harmonylite -cleanup on Node 1
# 3. Repeat for other nodes
# Events with new column will be partially applied to old-schema nodes
```

**Option B: Coordinated migration (any change)**
```bash
# 1. Initiate coordinated migration
harmonylite schema migrate \
    --name "add_user_email" \
    --version 6 \
    --ddl "ALTER TABLE users ADD COLUMN email TEXT"

# 2. System automatically:
#    - Pauses writes
#    - Drains pending changes
#    - Applies DDL on all nodes
#    - Resumes operation
```

---

## Open Questions

1. **DDL Replication**: Should we support automatic DDL replication (risky) or always require explicit coordination?

2. **Rollback Strategy**: How to handle partial migration failures? Snapshot restore vs. reverse DDL?

3. **Version Numbering**: Global monotonic version vs. per-table versions?

4. **Event Retention**: How long to keep pending events before discarding?

5. **Partial Policy Edge Cases**: What if a DELETE event references a column that doesn't exist locally?

---

## References

- [SQLite Schema Introspection](https://www.sqlite.org/pragma.html#pragma_table_info)
- [NATS KeyValue](https://docs.nats.io/nats-concepts/jetstream/key-value-store)
- [CockroachDB Schema Changes](https://www.cockroachlabs.com/docs/stable/online-schema-changes.html) (inspiration for coordination)
- [Vitess Schema Management](https://vitess.io/docs/user-guides/schema-changes/) (inspiration for policies)
