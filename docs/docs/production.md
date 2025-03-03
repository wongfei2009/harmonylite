# Production

This guide provides detailed instructions and best practices for deploying HarmonyLite in a production environment. It covers system requirements, deployment strategies, configuration recommendations, monitoring, and maintenance procedures to ensure a robust and reliable SQLite replication system.

## Table of Contents

- [System Requirements](#system-requirements)
- [Deployment Architecture](#deployment-architecture)
- [Network Configuration](#network-configuration)
- [Storage Configuration](#storage-configuration)
- [NATS Configuration](#nats-configuration)
- [Security Considerations](#security-considerations)
- [Monitoring and Alerts](#monitoring-and-alerts)
- [Backup and Recovery](#backup-and-recovery)
- [Scaling Strategies](#scaling-strategies)
- [Maintenance Procedures](#maintenance-procedures)
- [Troubleshooting](#troubleshooting)

## System Requirements

### Hardware Recommendations

For optimal performance in production environments, we recommend the following specifications per node:

| Component | Minimum | Recommended | Notes |
|-----------|---------|-------------|-------|
| CPU | 2 cores | 4+ cores | Additional cores benefit parallel processing |
| RAM | 2 GB | 4-8 GB | Larger for high-volume environments |
| Disk | 20 GB SSD | 50+ GB SSD | Faster storage improves snapshot performance |
| Network | 100 Mbps | 1 Gbps | Low latency is crucial for replication |

### Operating System Support

HarmonyLite runs on any Linux distribution with SQLite support. We specifically recommend:

- Ubuntu Server 20.04 LTS or newer
- Debian 11 or newer
- Amazon Linux 2 or newer

For development and testing, macOS is also supported.

### Dependencies

- SQLite 3.35.0 or newer
- Go 1.24 (only for building from source)
- libsqlite3-dev package (for building from source)

## Deployment Architecture

### Single Region Deployment

For applications hosted in a single region, we recommend a minimum of three nodes to ensure high availability:

```
                    ┌─────────────┐
                    │ Application │
                    └──────┬──────┘
                           │
             ┌─────────────┴─────────────┐
             │                           │
     ┌───────▼──────┐            ┌───────▼──────┐
     │ HarmonyLite  │◄──────────►│ HarmonyLite  │
     │   Node 1     │            │   Node 2     │
     └──────┬───────┘            └──────┬───────┘
            │                           │
            │         ┌─────────────────┘
            │         │
     ┌──────▼─────────▼─┐
     │  HarmonyLite     │
     │    Node 3        │
     └──────────────────┘
```

### Multi-Region Deployment

For globally distributed applications, deploy nodes in each region with cross-region NATS connectivity:

```
Region A                         Region B
┌────────────┐                   ┌────────────┐
│            │                   │            │
│ ┌────────┐ │                   │ ┌────────┐ │
│ │ Node 1 │◄│═══════════════════│►│ Node 4 │ │
│ └────┬───┘ │   Leaf Node       │ └────┬───┘ │
│      │     │   Connection      │      │     │
│      ▼     │                   │      ▼     │
│ ┌────────┐ │                   │ ┌────────┐ │
│ │ Node 2 │◄┼───┐               │ │ Node 5 │◄┼───┐
│ └────────┘ │   │               │ └────────┘ │   │
│            │   │ NATS Cluster  │            │   │ NATS Cluster
│            │   │ Connections   │            │   │ Connections
│ ┌────────┐ │   │               │ ┌────────┐ │   │
│ │ Node 3 │◄┼───┘               │ │ Node 6 │◄┼───┘
│ └────────┘ │                   │ └────────┘ │
│            │                   │            │
└────────────┘                   └────────────┘
```

Here's an annotated configuration example:

```toml
db_path="/path/to/regionA/node1.db"
node_id=1
seq_map_path="/path/to/regionA/node1-seq-map.cbor"

[replication_log]
shards=2  # Increased for cross-region traffic
max_entries=2048
replicas=3
compress=true  # Compression helps with WAN traffic

[snapshot]
enabled=true
store="s3"  # Using S3 for cross-region durability
interval=3600000

[snapshot.s3]
endpoint="s3.amazonaws.com"
path="harmonylite/snapshots"
bucket="your-backup-bucket"
use_ssl=true
access_key="your-access-key"
secret="your-secret-key"

[nats]
reconnect_wait_seconds=5  # Higher for cross-region resilience
```


### Role-Based Deployment

For specialized workloads, consider using role-based nodes:

- **Read-Only Nodes**: Set `publish=false` to prevent nodes from publishing changes
- **Write-Only Nodes**: Set `replicate=false` to have nodes ignore incoming changes
- **Full Nodes**: Default configuration that both publishes and replicates changes

## Network Configuration

### Required Ports

Ensure the following ports are open between HarmonyLite nodes:

| Port | Protocol | Purpose | Direction |
|------|----------|---------|-----------|
| 4222 | TCP | NATS connections | Bidirectional |
| 3010 | TCP | Prometheus metrics (optional) | Inbound |

### Network Latency Considerations

- Aim for inter-node latency below 50ms for optimal replication performance
- For nodes across regions, configure larger `max_entries` values to handle network delays
- Use `connect_retries` and `reconnect_wait_seconds` to handle temporary network interruptions

## Storage Configuration

### File System Recommendations

- Use an XFS or ext4 file system for optimal performance
- Enable journal features for data integrity
- Consider using LVM for snapshot capabilities
- Ensure `noatime` mount option is used to reduce I/O overhead

### Disk Layout

Organize your storage with separate volumes for:

1. **Operating System**: 20-50 GB
2. **HarmonyLite Binaries**: 1 GB
3. **Database Files**: Size according to your expected data volume + 50% buffer
4. **NATS JetStream Storage**: Size to accommodate your `max_entries` × shards × average message size

### Optimizing for I/O Performance

- Use SSDs or NVMe storage for database files
- Set proper I/O scheduler (e.g., `deadline` or `noop` for SSDs)
- Consider filesystem tuning parameters:
  ```bash
  # Increase the commit interval to reduce I/O operations
  echo 30 > /proc/sys/vm/dirty_ratio
  echo 10 > /proc/sys/vm/dirty_background_ratio
  ```

## NATS Configuration

### External vs. Embedded NATS

HarmonyLite can use either an embedded NATS server or an external NATS cluster:

- **Embedded NATS**: Simpler setup, recommended for small deployments (3-5 nodes)
- **External NATS**: Better for larger deployments, allows separate scaling and management

### External NATS Configuration

To use an external NATS cluster, specify the server URLs in your configuration:

```toml
[nats]
urls = [
    "nats://nats-server-1:4222",
    "nats://nats-server-2:4222",
    "nats://nats-server-3:4222"
]
```

### Embedded NATS Storage Configuration

For embedded NATS, customize the storage location to use a dedicated volume:

```toml
[nats]
server_config = "/etc/harmonylite/nats-server.conf"
```

With `/etc/harmonylite/nats-server.conf` containing:

```
jetstream {
  store_dir: "/data/nats/jetstream"
}
```

### Authentication

For a production environment, always enable authentication:

```toml
[nats]
# For basic username/password authentication
user_name = "harmonylite"
user_password = "secure-password-here"

# For NKEY authentication using a seed file
# Generate a user seed with: nk -gen user > user.seed
# Reference: https://docs.nats.io/running-a-nats-service/nats_admin/security/jwt#what-are-nkeys
seed_file = "/path/to/user.seed"
```

## Security Considerations

### Network Security

- Consider using a dedicated VLAN or VPC for node communication
- Implement network-level access controls (firewall rules, security groups)

### Running as a Non-Root User

Create a dedicated user for running HarmonyLite:

```bash
# Create a user for HarmonyLite
sudo useradd -r -s /bin/false harmonylite

# Create required directories
sudo mkdir -p /etc/harmonylite /var/lib/harmonylite /var/log/harmonylite

# Set permissions
sudo chown -R harmonylite:harmonylite /etc/harmonylite /var/lib/harmonylite /var/log/harmonylite
```

### Systemd Service

Create a systemd service file at `/etc/systemd/system/harmonylite.service`:

```ini
[Unit]
Description=HarmonyLite SQLite Replication Service
After=network.target

[Service]
User=harmonylite
Group=harmonylite
Type=simple
ExecStart=/usr/local/bin/harmonylite -config /etc/harmonylite/config.toml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

# Hardening options
ProtectSystem=full
PrivateTmp=true
NoNewPrivileges=true
ProtectHome=true
ProtectControlGroups=true
ProtectKernelModules=true

[Install]
WantedBy=multi-user.target
```

## Monitoring and Alerts

### Prometheus Metrics

Enable Prometheus metrics to monitor HarmonyLite performance:

```toml
[prometheus]
enable = true
bind = "0.0.0.0:3010"
namespace = "harmonylite"
subsystem = "replication"
```

### Key Metrics to Monitor

| Metric | Type | Description | Alert Threshold |
|--------|------|-------------|----------------|
| `harmonylite_published` | Counter | Number of published changes | N/A (trend) |
| `harmonylite_pending_publish` | Gauge | Changes waiting for publication | >1000 for >5 min |
| `harmonylite_count_changes` | Histogram | Latency to count changes | 95th percentile >500ms |
| `harmonylite_scan_changes` | Histogram | Latency to scan changes | 95th percentile >1s |

### Grafana Dashboard

Create a Grafana dashboard to visualize these metrics. You can find a sample JSON dashboard template in the [monitoring](https://github.com/wongfei2009/harmonylite/monitoring/) directory.

### Log Monitoring

Configure structured logging for production:

```toml
[logging]
verbose = false
format = "json"
```

Use a log aggregation system (such as Elasticsearch, Loki, or Graylog) to collect and analyze logs.

## Backup and Recovery

### Regular Snapshots

Configure automatic snapshots at an appropriate interval:

```toml
[snapshot]
enabled = true
interval = 3600000  # 1 hour in milliseconds
store = "s3"        # Or another supported storage type
```

### S3 Snapshot Configuration

For reliable backup storage, use S3 or S3-compatible storage:

```toml
[snapshot.s3]
endpoint = "s3.amazonaws.com"
path = "harmonylite/snapshots"
bucket = "your-backup-bucket"
use_ssl = true
access_key = "your-access-key"
secret = "your-secret-key"
```

### Recovery Procedures

To recover a node using the latest snapshot:

1. Stop the HarmonyLite service:
   ```bash
   sudo systemctl stop harmonylite
   ```

2. Remove the sequence map file to force snapshot restoration:
   ```bash
   sudo rm /var/lib/harmonylite/seq-map.cbor
   ```

3. Restart the service:
   ```bash
   sudo systemctl start harmonylite
   ```

## Scaling Strategies

### Horizontal Scaling

Add more nodes to increase capacity:

1. Deploy a new server with the HarmonyLite binary
2. Create a configuration file pointing to your NATS cluster
3. Start the service and it will automatically join the replication network

### Shard Configuration

For high-volume write environments, increase the number of shards:

```toml
[replication_log]
shards = 4      # Increase based on write volume
replicas = 3    # Maintain at least 3 replicas for redundancy
```

### Regional Read Replicas

For global deployments, configure read-only nodes in each region:

```toml
# Read-only node configuration
db_path = "/var/lib/harmonylite/data.db"
publish = false
replicate = true
```

## Maintenance Procedures

### Version Upgrades

To upgrade HarmonyLite:

1. Download the new version
2. Back up your configuration files
3. Stop the current instance:
   ```bash
   sudo systemctl stop harmonylite
   ```
4. Replace the binary
5. Check configuration compatibility
6. Start the service:
   ```bash
   sudo systemctl start harmonylite
   ```
7. Verify logs for successful startup

### Cleaning Up Artifacts

Periodically run cleanup to remove old change logs and optimize storage:

```bash
harmonylite -config /etc/harmonylite/config.toml -cleanup
```

### Schema Changes

When making schema changes to your SQLite database:

1. Stop all applications writing to the database
2. Apply schema changes on one node
3. Run cleanup to reset triggers:
   ```bash
   harmonylite -config /etc/harmonylite/config.toml -cleanup
   ```
4. Restart HarmonyLite on that node
5. Wait for changes to replicate
6. Repeat for other nodes
7. Resume application connections

## Troubleshooting

### Common Issues

| Issue | Possible Causes | Solution |
|-------|----------------|----------|
| Replication delays | Network latency, high write volume | Increase `shards`, check network connectivity |
| Out of disk space | Insufficient cleanup, high change volume | Run cleanup, increase `cleanup_interval` |
| Node cannot connect | NATS authentication, network issues | Check firewall rules, NATS credentials |
| Changes not replicating | Triggers not installed, SQLite version mismatch | Run cleanup and reinstall triggers |

### Diagnostic Commands

Check node status:
```bash
curl http://localhost:3010/metrics | grep harmonylite
```

View HarmonyLite logs:
```bash
journalctl -u harmonylite -f
```

Check NATS connectivity:
```bash
nats-top -s localhost -m 8222
```

### Recovery from Data Corruption

If a node's database becomes corrupted:

1. Stop the HarmonyLite service
2. Delete the database file and sequence map
3. Start the service - it will recover from the latest snapshot
