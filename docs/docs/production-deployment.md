# Production Deployment Guide

This guide provides comprehensive recommendations for deploying HarmonyLite in production environments. It covers hardware requirements, deployment architecture, security, monitoring, and maintenance procedures to ensure a reliable, high-performance system.

## System Requirements

### Hardware Recommendations

For production deployments, we recommend the following specifications per node:

| Component | Minimum | Recommended | High-Performance |
|-----------|---------|-------------|------------------|
| CPU | 2 cores | 4 cores | 8+ cores |
| RAM | 2 GB | 4-8 GB | 16+ GB |
| Storage | 20 GB SSD | 100+ GB SSD | 500+ GB NVMe SSD |
| Network | 100 Mbps | 1 Gbps | 10 Gbps |

Notes:
- CPU: Each additional core improves parallel processing capacity
- RAM: Larger for high-volume environments with many simultaneous connections
- Storage: SSD strongly recommended for performance; size based on your data volume and retention needs
- Network: Low latency is crucial for replication performance

### Operating System Recommendations

HarmonyLite works on most Linux distributions. We specifically recommend:

- Ubuntu Server 20.04/22.04 LTS
- Debian 11/12
- Amazon Linux 2/2023
- RHEL/CentOS 8 or newer

## Storage Configuration

### File System Recommendations

For production deployments:

- **File System**: XFS or ext4
- **Mount Options**: `noatime,nodiratime` to reduce I/O overhead
- **I/O Scheduler**: `none` for NVMe SSDs, `mq-deadline` for SATA SSDs
- **RAID**: RAID-10 for high-performance setups, RAID-1 for basic redundancy

### Disk Layout

Organize your storage with separate volumes:

1. **Operating System**: 20-50 GB
2. **HarmonyLite Binaries**: 1 GB
3. **Database Files**: Size according to your data volume plus 100% growth buffer
4. **NATS JetStream Storage**: Size to accommodate at least 2x your expected change volume

## Setting Up Production Nodes

### Creating a Dedicated User

Create a system user for running HarmonyLite:

```bash
# Create user and required directories
sudo useradd -r -s /bin/false harmonylite
sudo mkdir -p /etc/harmonylite /var/lib/harmonylite /var/log/harmonylite

# Set permissions
sudo chown -R harmonylite:harmonylite /etc/harmonylite /var/lib/harmonylite /var/log/harmonylite
sudo chmod 750 /etc/harmonylite /var/lib/harmonylite /var/log/harmonylite
```

### Configuration File

Create a production configuration file at `/etc/harmonylite/config.toml`. 

Be sure to review the [Configuration Reference](configuration-reference.md) for a complete list of all available options.

```toml
# Database settings
db_path = "/var/lib/harmonylite/data.db"
node_id = 1  # Unique per node
seq_map_path = "/var/lib/harmonylite/seq-map.cbor"
cleanup_interval = 30000  # 30 seconds

[replication_log]
shards = 4
max_entries = 2048
replicas = 3
compress = true

[snapshot]
enabled = true
store = "s3"  # Or another supported backend
interval = 3600000  # 1 hour

[snapshot.s3]
# See Configuration Reference for full S3 settings
endpoint = "s3.amazonaws.com"
bucket = "your-backup-bucket"
path = "harmonylite/snapshots"
use_ssl = true
access_key = "your-access-key"
secret = "your-secret-key"

[nats]
urls = ["nats://nats-server-1:4222", "nats://nats-server-2:4222", "nats://nats-server-3:4222"]
connect_retries = 10
reconnect_wait_seconds = 5

[prometheus]
enable = true
bind = "127.0.0.1:3010"  # Only bind to localhost if using a reverse proxy

[logging]
verbose = false  # Less verbose in production
format = "json"  # Structured logging
```

### Systemd Service

Create a systemd service file at `/etc/systemd/system/harmonylite.service`:

```ini
[Unit]
Description=HarmonyLite SQLite Replication Service
After=network.target
Documentation=https://github.com/wongfei2009/harmonylite

[Service]
User=harmonylite
Group=harmonylite
Type=simple
ExecStart=/usr/local/bin/harmonylite -config /etc/harmonylite/config.toml
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536
WorkingDirectory=/var/lib/harmonylite
StandardOutput=journal
StandardError=journal

# Hardening options
ProtectSystem=full
PrivateTmp=true
NoNewPrivileges=true
ProtectHome=true
ProtectControlGroups=true
ProtectKernelModules=true
ReadWritePaths=/var/lib/harmonylite /var/log/harmonylite

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable harmonylite
sudo systemctl start harmonylite
```

### Log Rotation

Since HarmonyLite logs to stdout/stderr by default when using systemd, the logs will be handled by the journal. To configure log retention:

```bash
sudo mkdir -p /etc/systemd/journald.conf.d/
sudo tee /etc/systemd/journald.conf.d/harmonylite.conf > /dev/null << EOF
[Journal]
MaxRetentionSec=14d
SystemMaxUse=1G
EOF

sudo systemctl restart systemd-journald
```

## Security Considerations

Security is critical for production deployments.

### Authentication and TLS

For configuring NATS authentication (Username/Password, NKeys) and TLS encryption, please refer to the [NATS Configuration Guide: Security](nats-configuration.md#security-configuration).

## Snapshot Configuration

Production environments should use a durable object storage backend for snapshots.

For detailed configuration of S3, SFTP, and WebDAV backends, please refer to the [Configuration Reference: Snapshot Settings](configuration-reference.md#snapshot-settings).

## Schema Changes

HarmonyLite supports **rolling schema upgrades** with automatic pause/resume behavior. When a schema mismatch is detected between nodes, replication pauses safely until schemas converge.

### Rolling Upgrade Workflow

You can upgrade your database schema one node at a time without stopping the entire cluster:

```bash
# 1. Stop the node
sudo systemctl stop harmonylite

# 2. Apply schema changes
sqlite3 /var/lib/harmonylite/data.db "ALTER TABLE users ADD COLUMN email TEXT"

# 3. Restart HarmonyLite (it will compute a new schema hash)
sudo systemctl start harmonylite

# 4. Repeat for remaining nodes
```

### How It Works

1. **Schema Hash Tracking**: Each replication message includes a schema hash computed from the table structure
2. **Mismatch Detection**: When a node receives a message with a different schema hash, it pauses replication
3. **Safe Pause**: The node NAKs messages with a delay, preserving message order in NATS
4. **Automatic Resume**: After the local schema is upgraded and HarmonyLite restarts, replication resumes automatically

### Monitoring Schema State

You can view the cluster-wide schema state via the NATS KV registry:

```bash
# Check if all nodes have the same schema (using nats CLI)
nats kv ls harmonylite-schema-registry
nats kv get harmonylite-schema-registry node-1
```

### Important Notes

- **During the migration window**: Nodes with older schemas will pause replication. This is expected behavior.
- **Order preservation**: Paused messages remain in NATS JetStream and are replayed after upgrade.
- **Change log recreation**: After altering a table, HarmonyLite automatically recreates the CDC triggers and change_log table with the new column structure.

For detailed design documentation, see [Schema Versioning Design](design/schema-versioning.md).

## Performance Tuning

Optimizing HarmonyLite depends on your specific workload and resource availability.

Please refer to the [Configuration Reference: Performance Tuning](configuration-reference.md#performance-tuning-configuration) for detailed advice on tuning `shards`, `scan_max_changes`, `cleanup_interval`, and other parameters.