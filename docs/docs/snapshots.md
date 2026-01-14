# Snapshot Management

This document details the operational mechanics, configuration tuning, and failure handling of HarmonyLite's snapshot system. For high-level concepts, see [Architecture](architecture.md).

:::tip Operator Guide
Snapshots are the primary mechanism for **disaster recovery** and **node provisioning**. Misconfiguration here can lead to slow recovery times or storage exhaustion.
:::

## Snapshot Lifecycle

The snapshot process is a multi-step operation designed to ensure data consistency without locking the database for writes.

```mermaid
stateDiagram-v2
    [*] --> Check: Interval Timer / Manual Trigger
    Check --> AcquireLease: Publish=True
    Check --> [*]: Publish=False
    
    state "Leader Election" as Election {
        AcquireLease --> Leader: Success
        AcquireLease --> Follower: Lease Held
        Follower --> [*]: Skip Upload
    }

    Leader --> Prepare
    
    state "Local Processing" as Local {
        Prepare --> VacuumInto: VACUUM INTO temp_dir
        VacuumInto --> Sanitize: Remove _change_log tables
        Sanitize --> Optimize: VACUUM (shrink size)
    }
    
    state "Upload Phase" as Upload {
        Optimize --> Uploading: Stream to Storage
        Uploading --> Verify: Check Hash/Size
        Verify --> Commit: Update Metadata
    }
    
    Commit --> Cleanup: Remove Temp Files
    Cleanup --> [*]
```

### 1. Atomic Creation (`VACUUM INTO`)
HarmonyLite uses SQLite's `VACUUM INTO 'filename'` command to create a transactionally consistent backup.
- **Non-Blocking**: Writers can continue to operate while the snapshot is being generated.
- **WAL Inclusion**: The checkpoint includes the current WAL state, ensuring no data is lost locally.

### 2. Sanitization
Before upload, the snapshot is "cleaned":
- **`__harmonylite_*` tables removal**: Change logs are stripped to reduce size. The snapshot represents the *state*, not the history.
- **Trigger drop**: Replication triggers are removed so they don't fire during the restore process (which would cause infinite loops).
- **Optimization**: A final `VACUUM` is run on the snapshot file to reclaim space from deleted cleanup rows.

### 3. Cleanup
Temporary files are stored in `os.TempDir()` with the pattern `harmonylite-snapshot-*`. The system attempts to remove these files immediately after upload.
- **Retry Logic**: If file deletion fails (e.g., file lock), the system retries 5 times with a 1-second backoff before logging an error.

## Snapshot Leader Election

When running a cluster of publishers (`publish = true`), it is inefficient and potentially dangerous for multiple nodes to upload snapshots simultaneously.

### The Lease Mechanism
HarmonyLite uses a distributed lease backed by the NATS Key-Value store (`snapshot-leader` key).

- **Lease Key**: `snapshot-leader`
- **Default TTL**: 30 seconds
- **Heartbeat**: Every 10 seconds (TTL / 3)

### Split-Brain Protection
Each node runs a background `electionLoop`.
1. **Acquire**: Attempts to create/update the key with its Node ID.
2. **Renew**: If it owns the key, it updates the timestamp.
3. **Loss**: If `AcquireLease` fails (e.g., NATS outage), the node immediately steps down (`isLeader.Store(false)`).

:::warning Implication
If NATS is temporarily unreachable, **no snapshots will be taken**. This is a safety feature to prevent split-brain uploads.
:::

## Negative Scenarios & Failure Modes

Understanding how the system behaves during failures is critical for operations.

### 1. Storage Backend Failures
If the configured storage (S3, SFTP, etc.) is unavailable during a snapshot upload:
- **Behavior**: The `SaveSnapshot()` function returns an error.
- **Retry**: There is **no immediate retry** for that specific snapshot interval. The system waits for the next configured `interval`.
- **Impact**: Recovery points become stale.
- **Monitoring**: Watch logs for `Unable to cleanup temp path` or `Upload failed`.

### 2. Disk Space Exhaustion
Snapshot creation requires temporary disk space roughly equal to the database size.
- **Error**: `no space left on device` during `VACUUM INTO`.
- **Result**: The snapshot is aborted.
- **Recovery**: The system cleans up partial files (best effort). If cleanup fails, you may see leftover `harmonylite-snapshot-*` directories in `/tmp`.

### 3. Leader Flapping
If network latency between the leader and NATS exceeds `leader_ttl`:
- **Symptom**: Leadership constantly switches between nodes.
- **Log**: frequent `Lost snapshot leadership` and `Became snapshot leader` messages.
- **Fix**: Increase `snapshot.leader_ttl` in configuration (e.g., from `30000` to `60000`).

### 4. "System Busy" (Overlap)
If a snapshot takes longer to generate/upload than the configured `interval`:
- **Error**: `ErrPendingSnapshot` ("system busy capturing snapshot").
- **Behavior**: The new snapshot attempt is skipped.
- **Prevention**: Ensure `interval` >> (Generation Time + Upload Time).

### 5. Corrupted Snapshot Download
If a downloaded snapshot is corrupted (truncated file, bad checksum):
- **Detection**: SQLite `RestoreFrom` will fail or the database will be unopenable.
- **Recovery**:
    - **Automatic**: The node logs the error and effectively restarts the initialization process.
    - **Manual Intervention**: Stop the node, delete the local `data.db` and `seq-map.cbor`, and restart.

## Configuration Tuning

### Basic Schedule
```toml
[snapshot]
enabled = true
interval = 3600000        # 1 hour (Default). Set lower for lower RPO.
```

### Leadership Tuning
For high-latency networks or heavy-load leaders:
```toml
[snapshot]
leader_ttl = 60000        # Increase to 60s to prevent flapping
```

### Storage Backends

#### S3 (AWS/MinIO)
Supports standard AWS SDK authentication (Env vars, IAM roles).
```toml
[snapshot.s3]
endpoint = "s3.us-east-1.amazonaws.com" # or minio:9000
bucket = "backups"
path = "prod-cluster"     # Prefix for keys
region = "us-east-1"
force_path_style = false  # Set true for MinIO
```

#### SFTP
Useful for "air-gapped" backups to a secure host.
```toml
[snapshot.sftp]
# format: sftp://user:pass@host:port/path
url = "sftp://backup:secret@10.0.0.5:22/mnt/backups"
```

## Restoration & Recovery

### How to Force a Restore
To force a node to re-download the latest snapshot:

1. **Stop the service**: `systemctl stop harmonylite`
2. **Clear data**: `rm /var/lib/harmonylite/data.db`
3. **Clear sequence map**: `rm /var/lib/harmonylite/seq-map.cbor`
    - *Critical*: Removing the sequence map forces the node to check for snapshots. If you verify the DB but keep the map, it will try to replay streams instead.
4. **Start service**: `systemctl start harmonylite`

### Verifying Integrity
You can verify a snapshot integrity manually:
```bash
sqlite3 snapshot.db "PRAGMA integrity_check;"
```