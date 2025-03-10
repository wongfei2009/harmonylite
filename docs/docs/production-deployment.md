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

Create a production configuration file at `/etc/harmonylite/config.toml`:

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

Customize configuration options for your specific environment.

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

### Authentication

For NATS authentication in production:

```toml
[nats]
# Option 1: Username/Password
user_name = "harmonylite"
user_password = "secure-random-password"

# Option 2: NKey authentication (preferred)
seed_file = "/etc/harmonylite/nkeys/user.nkey"
```

### TLS Configuration

For TLS-secured connections to NATS:

```toml
[nats]
urls = ["tls://nats-server-1:4222"]
seed_file = "/etc/harmonylite/nkeys/user.nkey"
ca_file = "/etc/harmonylite/tls/ca.pem"
cert_file = "/etc/harmonylite/tls/client-cert.pem"
key_file = "/etc/harmonylite/tls/client-key.pem"
```

## Snapshot Configuration

For robust snapshot configurations in production:

```toml
[snapshot]
enabled = true
interval = 3600000  # 1 hour
store = "s3"  # S3, WebDAV, or SFTP recommended for production

[snapshot.s3]
endpoint = "s3.amazonaws.com"
path = "harmonylite/snapshots"
bucket = "your-backup-bucket"
use_ssl = true
access_key = "your-access-key"
secret = "your-secret-key"
```

For SFTP or WebDAV storage:

```toml
[snapshot.sftp]
url = "sftp://username:password@sftp-server:22/path/to/store/snapshots"

# OR

[snapshot.webdav]
url = "https://webdav-server/path?dir=/snapshots&login=username&secret=password"
```

## Schema Changes

When making schema changes to your SQLite database:

1. Stop applications writing to the database
2. Apply schema changes on one node
3. Run cleanup to reset triggers:
   ```bash
   harmonylite -config /etc/harmonylite/config.toml -cleanup
   ```
4. Restart HarmonyLite on that node:
   ```bash
   sudo systemctl restart harmonylite
   ```
5. Repeat for other nodes
6. Resume application connections

## Performance Tuning

### Replication Tuning

Adjust these parameters based on your workload:

```toml
# High write throughput
[replication_log]
shards = 8
max_entries = 4096
```