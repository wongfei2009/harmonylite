# Automatic Pending Event Replay Design

**Status:** Draft  
**Author:** TBD  
**Created:** 2025-01-20  
**Related:** [Schema Versioning Design](../docs/design/schema-versioning.md)

## Overview

This document refines the schema versioning design to add automatic replay of pending events on node restart, eliminating the need for manual `harmonylite schema replay` commands.

## Design Decisions

| Aspect | Decision |
|--------|----------|
| When to replay | On node restart only (before normal replication starts) |
| If events still don't match schema | Keep queued + log warning |
| If replay fails (schema matches but apply fails) | Move to dead-letter table for manual inspection |
| Dead-letter management | Query table directly with sqlite3 (no CLI command) |

## Changes from Original Design

### Removed

- `harmonylite schema replay --table <name>` CLI command (no longer needed)

### Added

- Automatic replay during startup sequence
- Dead-letter table for failed events
- Dead-letter count in `harmonylite schema pending` output

---

## Automatic Replay on Startup

Instead of requiring manual `harmonylite schema replay`, pending events are automatically replayed during node startup:

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

**Key points:**
- Replay happens automatically, no CLI command needed
- Blocks startup until replay completes (ensures ordering)
- Clear logging for each outcome

---

## Dead-Letter Table Schema

Events that fail to apply (schema matches but apply fails) are moved to a dead-letter table:

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

**Fields:**
- `error_message`: Captures the specific failure reason (constraint violation, etc.)
- `queued_at` vs `failed_at`: Helps understand how long the event was pending
- `source_node_id`: Useful for debugging cross-node issues

Users can query directly:
```bash
sqlite3 mydb.db "SELECT table_name, error_message, datetime(failed_at, 'unixepoch') FROM __harmonylite__dead_letter_events"
```

---

## Replay Logic

The replay function processes pending events in order, with three possible outcomes per event:

```go
// db/pending_events.go

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

**Three outcomes:**
1. **Schema still mismatched** - stays in pending queue, logged as warning
2. **Schema matches, apply succeeds** - removed from pending
3. **Schema matches, apply fails** - moved to dead-letter table

---

## Updated CLI Commands

**Removed:**
```bash
# REMOVED: harmonylite schema replay --table users
```

**Updated `harmonylite schema pending` output:**
```bash
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

---

## Startup Logging

```
INFO  Starting HarmonyLite v1.2.0
INFO  Initializing schema cache for 3 tables
INFO  Replaying pending events...
INFO  Replayed pending events                    count=42
WARN  Pending events still incompatible          count=5 tables=["orders"]
WARN  Events moved to dead-letter table          count=2 tables=["users"]
INFO  Starting NATS replication...
```

---

## Implementation Notes

This design modifies the following sections of the original schema versioning design:

1. **Section 7 (Queued Event Replay)**: Replace manual replay with automatic startup replay
2. **Section 8 (CLI Commands)**: Remove `schema replay` command, update `schema pending` output
3. **Implementation Phases**: Update Phase 3 tasks to reflect automatic replay

The dead-letter table should be created alongside `__harmonylite__pending_events` in Phase 3.
