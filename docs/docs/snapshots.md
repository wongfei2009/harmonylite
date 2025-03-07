# Snapshot Management

This document explains HarmonyLite's snapshot system, which is critical for efficient node recovery and synchronization. Snapshots provide a way to quickly bootstrap new nodes or resynchronize nodes that have been offline for extended periods.

## Why Snapshots Matter

In a distributed system like HarmonyLite, snapshots serve several essential purposes:

1. **Efficient Recovery**: New or recovering nodes can start from a recent state rather than replaying all historical changes
2. **Performance Optimization**: Reduces the need to store and process all historical change logs
3. **Storage Management**: Allows for cleanup of old change records that have been superseded
4. **Disaster Recovery**: Provides point-in-time backups of database state

Without snapshots, nodes would need to replay the entire history of changes from the beginning of time, which becomes impractical as the system ages.

## How Snapshots Work

HarmonyLite's snapshot system operates through several coordinated components:

### Sequence Map

The Sequence Map is a critical component that tracks processed message sequences across JetStream shards:

- **Purpose**: Records the last processed message sequence for each stream
- **Implementation**: Stored as a key-value map (stream name → sequence number) serialized with CBOR
- **Location**: Specified via `seq_map_path` in the configuration
- **Recovery Role**: Acts as a database checkpoint to avoid reprocessing messages

```mermaid
graph LR
    SM[Sequence Map<br>seq-map.cbor] -->|Tracks| SS[Streams State]
    SS -->|Determines| NG{Need Snapshot?}
    NG -->|Yes| RS[Restore Snapshot]
    NG -->|No| CP[Continue Processing]
    SM -->|Enables| WR[Warm Restarts]
    WR -->|Skip| AP[Already Processed<br>Messages]
```

### Snapshot Creation Process

Snapshots are created based on configured criteria or manual triggers:

```mermaid
graph TB
    A[Monitor Sequence Numbers] --> B{Threshold Reached?}
    B -->|Yes| C[Create Temp Directory]
    B -->|No| A
    C --> D["VACUUM INTO (Temp Copy)"]
    D --> E[Remove Triggers & Logs]
    E --> F[Optimize with VACUUM]
    F --> G[Upload Snapshot]
    G --> H[Update Sequence Map]
    H --> A
```

The process involves:

1. **Determining Need**: Checking if sequence numbers have advanced enough to warrant a snapshot
2. **Creating Clean Copy**: Using SQLite's `VACUUM INTO` to create a copy without WAL files
3. **Cleaning Up**: Removing HarmonyLite-specific tables and triggers for a clean snapshot
4. **Optimizing**: Running `VACUUM` to optimize storage
5. **Storing**: Uploading the snapshot to the configured storage backend