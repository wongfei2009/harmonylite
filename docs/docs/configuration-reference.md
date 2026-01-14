# Configuration Reference

This document provides a comprehensive reference for all HarmonyLite configuration options. Use this guide to understand available settings and fine-tune your deployment.

## Configuration File Format

HarmonyLite uses a TOML configuration file format. By default, it looks for `config.toml` in the current directory, but you can specify a different path with the `-config` command-line parameter.

:::warning Environment Variables
**Environment Variables**: HarmonyLite configuration is strictly controlled via the TOML file and command-line flags. Environment variables (e.g., `HARMONYLITE_DB_PATH`) are **not** supported and will be ignored.
:::

## Basic Configuration

```toml
# Database path (required)
db_path = "/path/to/your.db"

# Unique node identifier (required, integer)
node_id = 1

# Path to persist sequence map (required)
seq_map_path = "/path/to/seq-map.cbor"

# Enable/disable publishing changes (optional, default: true)
publish = true

# Enable/disable replicating changes (optional, default: true)
replicate = true

# Number of maximum rows to process per batch (optional, default: 512)
scan_max_changes = 512

# Cleanup interval in milliseconds (optional, default: 5000)
cleanup_interval = 5000

# Sleep timeout in milliseconds for serverless environments (optional, default: 0, disabled)
sleep_timeout = 0

# Polling interval in milliseconds (optional, default: 0, disabled)
# Only useful for broken or buggy file system watchers
polling_interval = 0
```

## Replication Log Settings

```toml
[replication_log]
# Number of shards for parallel processing (optional, default: 1)
shards = 1

# Maximum entries per stream (optional, default: 1024)
max_entries = 1024

# Number of stream replicas for fault tolerance (optional, default: 1)
replicas = 3

# Enable zstd compression for change logs (optional, default: true)
compress = true

# Update existing stream if configurations don't match (optional, default: false)
update_existing = false
```

## Snapshot Settings

```toml
[snapshot]
# Enable snapshot support (optional, default: true)
enabled = true

# Snapshot storage backend (required if enabled)
# Options: "nats", "s3", "webdav", "sftp"
store = "nats"

# Snapshot interval in milliseconds (optional, default: 0, disabled)
# If there was a snapshot saved within interval range due to log threshold triggers, 
# then new snapshot won't be saved
interval = 3600000

# Leader election TTL in milliseconds (optional, default: 30000)
# Used when multiple nodes have publish=true to coordinate who uploads snapshots
# Only one node will be elected as the snapshot leader at a time
leader_ttl = 30000
```

## NATS Configuration

```toml
[nats]
# List of NATS server URLs (optional, if empty uses embedded server)
urls = ["nats://localhost:4222"]

# Prefix for change log subjects (optional, default: "harmonylite-change-log")
subject_prefix = "harmonylite-change-log"

# Prefix for JetStream streams (optional, default: "harmonylite-changes")
stream_prefix = "harmonylite-changes"

# Bind address for embedded NATS server (optional, default: ":-1" which means random port)
bind_address = ":-1"

# Path to custom NATS server config file (optional)
server_config = "/path/to/nats-server.conf"

# Connection retries for external servers (optional, default: 5)
connect_retries = 5

# Delay between reconnect attempts in seconds (optional, default: 2)
reconnect_wait_seconds = 2

# Authentication username (optional)
user_name = "harmonylite"

# Authentication password (optional)
user_password = "secure-password-here"

# Path to NKEY seed file (optional)
seed_file = "/path/to/user.seed"

# TLS configuration (optional)
ca_file = "/path/to/ca.pem"
cert_file = "/path/to/client-cert.pem"
key_file = "/path/to/client-key.pem"
```

## NATS Snapshot Storage

```toml
[snapshot.nats]
# Number of snapshot replicas (optional, default: 1)
replicas = 2

# Bucket name for object storage (optional)
bucket = "harmonylite-snapshots"
```

## S3 Snapshot Storage

```toml
[snapshot.s3]
# S3 endpoint (required for S3 storage)
endpoint = "s3.amazonaws.com"

# Path prefix inside bucket (optional)
path = "harmonylite/snapshots"

# Bucket name (required for S3 storage)
bucket = "your-backup-bucket"

# Use SSL for connections (optional, default: false)
use_ssl = true

# Access key for authentication (required for S3 storage)
access_key = "your-access-key"

# Secret key for authentication (required for S3 storage)
secret = "your-secret-key"

# Session token (optional)
session_token = ""
```

## WebDAV Snapshot Storage

```toml
[snapshot.webdav]
# WebDAV server URL with parameters (required for WebDAV storage)
url = "https://<webdav_server>/<web_dav_path>?dir=/snapshots/path/for/harmonylite&login=<username>&secret=<password>"
```

## SFTP Snapshot Storage

```toml
[snapshot.sftp]
# SFTP server URL with credentials (required for SFTP storage)
url = "sftp://<user>:<password>@<sftp_server>:<port>/path/to/save/snapshot"
```

## Logging Configuration

```toml
[logging]
# Enable verbose logging (optional, default: false)
verbose = true

# Log format (optional, default: "console")
# Options: "console", "json"
format = "json"
```

## Prometheus Metrics

```toml
[prometheus]
# Enable Prometheus metrics (optional, default: false)
enable = true

# Metrics HTTP listener address (optional, default: "0.0.0.0:3010")
bind = "0.0.0.0:3010"

# Metrics namespace (optional, default: "harmonylite")
namespace = "harmonylite"

# Metrics subsystem (optional, default: "")
subsystem = ""
```

## Health Check Endpoint

```toml
[health_check]
# Enable health check endpoint (optional, default: false)
enable = false

# Health check HTTP listener address (optional, default: "0.0.0.0:8090")
bind = "0.0.0.0:8090"

# Health check endpoint path (optional, default: "/health")
path = "/health"

# Include detailed information in response (optional, default: true)
detailed = true
```

## Performance Tuning Configuration

Optimizing HarmonyLite depends on your specific workload and resource availability. Here are key parameters to tune:

### Throughput vs. Latency
- **`scan_max_changes`**: Controls batch size for processing changes.
    - **Increase (e.g., 2048)**: Higher throughput, better for bulk updates, but may increase memory usage per batch.
    - **Decrease (e.g., 100)**: Lower latency per batch, "smoother" processing for real-time needs, but lower overall throughput.

### Resource Usage
- **`cleanup_interval`**: Frequency of cleaning up old log entries.
    - **Increase (e.g., 60000)**: Reduces CPU overhead from frequent deletions, but uses more disk space for temporary logs.
    - **Decrease (e.g., 1000)**: Keeps disk usage minimal, but burns more CPU cycles.

### Consistency & Safety
- **`leader_ttl`**: Time-to-live for snapshot leader lease.
    - **Default (30s)**: Balanced for stable networks.
    - **Increase**: Use in high-latency or unstable networks to prevent "flapping" leadership.
    - **Decrease**: Faster failover if a leader crashes, but higher risk of split-brain in poor networks.
- **`replicas` (Replication Log & Snapshot)**:
    - **Production**: Always set `replicas >= 3` for fault tolerance.
    - **Development**: `replicas = 1` saves storage and overhead.

## Common Scenarios

### 1. Local Development (Minimal)
Optimized for low overhead and quick setup. Disables heavy durability features.

```toml
# Use temporary paths
db_path = "/tmp/harmonylite-dev.db"
node_id = 1
seq_map_path = "/tmp/harmonylite-dev-seq.cbor"

# No compression needed locally
[replication_log]
shards = 1
max_entries = 128
replicas = 1
compress = false 

# Use embedded NATS with default random port
[nats]
bind_address = ":-1" 

# Disable snapshots for quick iteration
[snapshot]
enabled = false
```

### 2. High Performance (Throughput Optimized)
Tuned for systems with high write volume.

```toml
# Larger batch sizes and cleanup intervals
scan_max_changes = 2048
cleanup_interval = 60000

[replication_log]
# Increase shards to parallelize processing
shards = 4
max_entries = 4096
# Enable compression to save network bandwidth
compress = true
```

### 3. High Availability (Production)
Focuses on fault tolerance and data safety.

```toml
# connect to external NATS cluster
[nats]
urls = ["nats://nats-1:4222", "nats://nats-2:4222", "nats://nats-3:4222"]
connect_retries = 10
reconnect_wait_seconds = 5

# Ensure redundancy
[replication_log]
replicas = 3
update_existing = true

# Robust snapshotting with object storage
[snapshot]
enabled = true
store = "s3"
interval = 3600000 # 1 hour
leader_ttl = 30000

[snapshot.s3]
endpoint = "s3.us-east-1.amazonaws.com"
bucket = "prod-backups"
# ... credentials ...

# Monitoring is critical for HA
[prometheus]
enable = true
bind = "0.0.0.0:3010"

[health_check]
enable = true
detailed = true
```

### 4. Edge/Replica Node (Read-Only)
A node that receives data but never writes back to the cluster.

```toml
node_id = 50
# Important: Disable publishing to avoid accidental writes propagating
publish = false
replicate = true

[replication_log]
# Lower resource usage for read-only
shards = 1
replicas = 2
```

## Command-Line Options

In addition to the configuration file, HarmonyLite accepts several command-line parameters:

| Parameter | Description |
|-----------|-------------|
| `-config` | Path to configuration file |
| `node-id` | Override node ID from config file |
| `-cluster-addr` | Embedded NATS server cluster address |
| `-cluster-peers` | Comma-separated list of peer URLs |
| `-leaf-servers` | Comma-separated list of leaf servers |
| `-cleanup` | Clean up triggers and log tables |
| `-save-snapshot` | Force snapshot creation |
| `-pprof` | Enable profiling server on specified address |
| `-help` | Display help information |

Example usage:
```bash
harmonylite -config /etc/harmonylite/config.toml -cluster-addr 127.0.0.1:4222 -cluster-peers nats://127.0.0.1:4223/
```