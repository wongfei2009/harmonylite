# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with HarmonyLite deployments. It covers installation problems, replication issues, performance bottlenecks, and recovery procedures.

## Diagnostic Tools

Before diving into specific issues, familiarize yourself with these diagnostic tools:

### Log Analysis

HarmonyLite logs provide valuable troubleshooting information. When using systemd, logs are sent to the journal. Enable verbose logging temporarily by modifying your config:

```toml
[logging]
verbose = true
format = "json"  # or "console" for human-readable format
```

Access logs using journalctl:

```bash
# View all logs for the HarmonyLite service
journalctl -u harmonylite

# View recent logs
journalctl -u harmonylite -n 100

# Follow logs in real-time
journalctl -u harmonylite -f
```

### Prometheus Metrics

Enable Prometheus metrics to monitor performance:

```toml
[prometheus]
enable = true
bind = "0.0.0.0:3010"
```

Access metrics at `http://<node-ip>:3010/metrics`

### Health Check Endpoint

HarmonyLite provides a health check HTTP endpoint that can be used for monitoring the status of nodes. Enable it in your configuration:

```toml
[health_check]
enable = true
bind = "0.0.0.0:8090"
path = "/health"
detailed = true
```

Access the health status at `http://<node-ip>:8090/health`

This health check can be integrated with container orchestration systems like Docker and Kubernetes for automated monitoring and failover. See the [Health Check documentation](./health-check.md) for more details.

### Available Metrics

HarmonyLite exposes the following metrics that can be used for monitoring and troubleshooting:

#### Database Metrics

|Metric|Type|Description|
|---|---|---|
|`published`|Counter|Number of database change rows that have been published to the NATS stream|
|`pending_publish`|Gauge|Number of rows that are pending to be published, which can indicate a backlog|
|`count_changes`|Histogram|Latency (in microseconds) for counting changes in the database|
|`scan_changes`|Histogram|Latency (in microseconds) for scanning change rows in the database|

#### Performance Indicators

- **High `pending_publish` values** indicate that HarmonyLite is experiencing delays in propagating changes.
- **Increasing `count_changes` or `scan_changes` latencies** may indicate database performance issues.
- **Low `published` rate** compared to write activity could indicate replication issues.

### Understanding HarmonyLite Metrics

HarmonyLite uses a change data capture (CDC) mechanism to track and replicate database changes. The metrics help monitor this process:

1. **Change Detection**: When database changes occur, HarmonyLite detects them and marks them as pending in a change log table.
    
2. **Change Publishing**: The pending changes are published to NATS streams, and the `published` counter increases.
    
3. **Replication**: Other nodes consume these published changes and apply them to their local databases.
    

Monitoring these metrics provides insights into the health of this process. For example:

- A consistently high `pending_publish` value could indicate network issues or that consumers are not keeping up with the change rate.
- If `count_changes` and `scan_changes` latencies increase, it might indicate that the SQLite database is under heavy load.

### NATS Monitoring

Check NATS server status:

```bash
# If using embedded NATS
curl http://localhost:8222/varz
curl http://localhost:8222/jsz

# List streams
curl http://localhost:8222/jsz?streams=1

```

### SQLite Analysis

Examine the SQLite database directly:

```bash
sqlite3 /path/to/your.db
```

Useful SQLite commands:
```sql
-- Check if triggers are installed
SELECT name FROM sqlite_master WHERE type='trigger' AND name LIKE '__harmonylite%';

-- Check change log tables
SELECT name FROM sqlite_master WHERE type='table' AND name LIKE '__harmonylite%';

-- Count pending changes
SELECT COUNT(*) FROM __harmonylite___change_log_global;
```

### Performance Profiling with pprof

HarmonyLite includes Go's built-in performance profiling which can be enabled with the `-pprof` flag to diagnose performance issues:

```bash
# Start HarmonyLite with profiling enabled on port 6060
./harmonylite -config /path/to/config.toml -pprof "127.0.0.1:6060"
```

Once enabled, you can access the following profiling endpoints:

- **Overview**: `http://127.0.0.1:6060/debug/pprof/`
- **CPU Profile**: `http://127.0.0.1:6060/debug/pprof/profile` (runs for 30 seconds by default)
- **Heap Memory Profile**: `http://127.0.0.1:6060/debug/pprof/heap`
- **Goroutine Stack Traces**: `http://127.0.0.1:6060/debug/pprof/goroutine`
- **Thread Creation Profile**: `http://127.0.0.1:6060/debug/pprof/threadcreate`
- **Blocking Profile**: `http://127.0.0.1:6060/debug/pprof/block`
- **Execution Trace**: `http://127.0.0.1:6060/debug/pprof/trace`

For more advanced analysis, use the Go pprof tool:

```bash
# CPU profiling
go tool pprof http://127.0.0.1:6060/debug/pprof/profile

# Memory profiling
go tool pprof http://127.0.0.1:6060/debug/pprof/heap

# For a 5-second CPU profile:
go tool pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=5
```

Inside the pprof interactive shell:
- `top`: Show top functions by usage
- `web`: Generate a graph visualization (requires Graphviz)
- `list [function]`: Show source code with profiling data

**Note**: Use profiling carefully in production environments as it exposes internal details about your application and may impact performance.

## Common Issues and Solutions

### Installation and Setup

#### Problem: HarmonyLite Fails to Start

**Symptoms**:
- Service fails to start
- "command not found" errors
- Permission denied errors

**Potential Causes and Solutions**:

1. **Binary not executable**:
   ```bash
   chmod +x /path/to/harmonylite
   ```

2. **Missing dependencies**:
   ```bash
   ldd /path/to/harmonylite
   # Install any missing dependencies
   ```

3. **Permission issues**:
   ```bash
   # Check file ownership
   ls -la /path/to/harmonylite
   
   # Check directory permissions
   ls -la /var/lib/harmonylite
   
   # Fix permissions
   chown harmonylite:harmonylite /var/lib/harmonylite
   chmod 750 /var/lib/harmonylite
   ```

4. **Config file problems**:
   ```bash
   # Validate config manually
   cat /path/to/config.toml
   ```

#### Problem: Configuration Validation Errors

**Symptoms**:
- "Invalid configuration" errors
- Service starts but exits immediately

**Solutions**:

1. Verify TOML syntax is valid
2. Check that all required fields are present
3. Ensure paths exist and are accessible
4. Validate that `node_id` is unique within the cluster

### Replication Issues

#### Problem: Changes Not Replicating

**Symptoms**:
- Changes made on one node are not appearing on other nodes
- Replication metrics show no activity

**Potential Causes and Solutions**:

1. **NATS connectivity issues**:
   ```bash
   # Check NATS status
   curl http://localhost:8222/varz
   
   # Test connection from other nodes
   telnet <nats-server-ip> 4222
   ```

2. **Triggers not installed**:
   ```sql
   -- Check triggers
   SELECT name FROM sqlite_master WHERE type='trigger' AND name LIKE '__harmonylite%';
   
   -- Reinstall triggers
   -- Exit SQLite and run:
   harmonylite -config /path/to/config.toml -cleanup
   -- Then restart HarmonyLite
   ```

3. **Change logs not being created**:
   ```sql
   -- Make a test change with trusted_schema enabled
   PRAGMA trusted_schema = ON;
   INSERT INTO test_table (name) VALUES ('test');
   
   -- Check if it appears in change log
   SELECT * FROM __harmonylite__test_table_change_log ORDER BY id DESC LIMIT 1;
   ```

4. **Publishing disabled**:
   ```
   # Check config.toml for:
   publish = false  # Should be true for nodes that need to send changes
   ```

5. **NATS stream not created**:
   ```bash
   # Check if streams exist
   curl http://localhost:8222/jsz?streams=1
   
   # Recreate streams
   # First stop HarmonyLite, then restart with clean state
   rm /path/to/seq-map.cbor
   # Restart HarmonyLite
   ```

#### Problem: High Replication Latency

**Symptoms**:
- Changes take a long time to propagate
- High `pending_publish` metrics

**Solutions**:

1. **Increase shards**:
   ```toml
   [replication_log]
   shards = 4  # Increase from default
   ```

2. **Enable compression**:
   ```toml
   [replication_log]
   compress = true
   ```

3. **Check network latency**:
   ```bash
   ping <other-node-ip>
   ```

4. **Monitor disk I/O**:
   ```bash
   iostat -x 1
   ```

5. **Adjust cleanup interval**:
   ```toml
   # Decrease to cleanup more frequently
   cleanup_interval = 30000  # 30 seconds
   ```

### Database Issues

#### Problem: Database Locks

**Symptoms**:
- "database is locked" errors
- Operations timing out
- Replication stalls

**Solutions**:

1. **Check for long-running transactions**:
   ```sql
   PRAGMA busy_timeout = 30000;  -- Set in your application
   ```

2. **Use WAL journal mode**:
   ```sql
   PRAGMA journal_mode = WAL;  -- Set in your application
   ```

3. **Check for other processes accessing the database**:
   ```bash
   lsof | grep your.db
   ```

4. **Verify SQLite version**:
   ```bash
   sqlite3 --version
   # Should be 3.35.0 or newer
   ```

5. **Consider timeout settings**:
   ```bash
   # Add to application connection string
   ?_timeout=30000&_journal_mode=WAL
   ```

#### Problem: Database Corruption

**Symptoms**:
- "malformed database" errors
- Unexpected query results
- Application crashes

**Solutions**:

1. **Check database integrity**:
   ```sql
   PRAGMA integrity_check;
   ```

2. **Restore from snapshot**:
   ```bash
   # Stop HarmonyLite
   systemctl stop harmonylite
   
   # Remove corrupt database
   rm /path/to/your.db
   
   # Restart to trigger recovery
   systemctl start harmonylite
   ```

3. **Recover from backup**:
   ```bash
   # Restore from backup
   cp /path/to/backup.db /path/to/your.db
   
   # Remove sequence map to force reinitialization
   rm /path/to/seq-map.cbor
   
   # Restart HarmonyLite
   systemctl start harmonylite
   ```

### Snapshot and Recovery

#### Problem: Snapshot Creation Fails

**Symptoms**:
- "Failed to create snapshot" errors
- No snapshots appearing in storage
- `snapshot_age` metric keeps increasing

**Solutions**:

1. **Check storage connectivity**:
   ```bash
   # Test S3 access
   aws s3 ls s3://your-bucket/
   
   # Test WebDAV
   curl -u username:password https://webdav.example.com/
   ```

2. **Verify permissions**:
   ```bash
   # For local file storage
   ls -la /path/to/snapshot/dir
   
   # For S3
   aws s3 ls s3://your-bucket/ --debug
   ```

3. **Ensure enough disk space**:
   ```bash
   df -h
   ```

4. **Force snapshot creation**:
   ```bash
   harmonylite -config /path/to/config.toml -save-snapshot
   ```

5. **Check storage configuration**:
   ```toml
   [snapshot]
   enabled = true
   store = "s3"  # Verify this matches your credentials
   
   [snapshot.s3]
   # Verify all credentials are correct
   ```

#### Problem: Recovery from Snapshot Fails

**Symptoms**:
- "Failed to restore snapshot" errors
- Service fails to start after deleting database
- Inconsistent state after recovery

**Solutions**:

1. **Check sequence map**:
   ```bash
   # Remove sequence map to force full recovery
   rm /path/to/seq-map.cbor
   ```

2. **Verify snapshot access**:
   ```bash
   # For S3
   aws s3 ls s3://your-bucket/harmonylite/snapshots/
   ```

3. **Try manual restore**:
   ```bash
   # Download snapshot manually
   aws s3 cp s3://your-bucket/harmonylite/snapshots/latest.db /tmp/
   
   # Replace database
   cp /tmp/latest.db /path/to/your.db
   
   # Fix permissions
   chown harmonylite:harmonylite /path/to/your.db
   
   # Remove sequence map
   rm /path/to/seq-map.cbor
   
   # Restart
   systemctl start harmonylite
   ```

4. **Check logs for specific errors**:
   ```bash
   journalctl -u harmonylite | grep "snapshot"
   ```

### Performance Issues

#### Problem: High CPU Usage

**Symptoms**:
- CPU consistently above 70%
- Slow response times
- Process using excessive resources

**Solutions**:

1. **Profile the process**:
   ```bash
   top -p $(pgrep harmonylite)
   ```

2. **Check if compression is causing overhead**:
   ```toml
   # Try disabling compression temporarily
   [replication_log]
   compress = false
   ```

3. **Adjust shard count**:
   ```toml
   # If too high, reduce:
   [replication_log]
   shards = 2  # Start low and increase as needed
   ```

4. **Monitor change volume**:
   ```bash
   # Check Prometheus metrics
   curl http://localhost:3010/metrics | grep harmonylite_published
   ```

5. **Consider hardware upgrade** if consistently high

#### Problem: Memory Leaks

**Symptoms**:
- Steadily increasing memory usage
- Eventually crashes with out-of-memory errors
- Degraded performance over time

**Solutions**:

1. **Monitor memory usage**:
   ```bash
   ps -o pid,user,%mem,rss,command -p $(pgrep harmonylite)
   ```

2. **Set memory limits in systemd**:
   ```ini
   # In /etc/systemd/system/harmonylite.service
   [Service]
   MemoryLimit=512M
   ```

3. **Restart periodically** if needed:
   ```bash
   # In crontab
   0 4 * * * systemctl restart harmonylite
   ```

4. **Update to latest version** as memory leaks are often fixed in updates

### NATS Issues

#### Problem: NATS Connection Failures

**Symptoms**:
- "Failed to connect to NATS" errors
- Intermittent disconnections
- Stream creation failures

**Solutions**:

1. **Check NATS server status**:
   ```bash
   curl http://localhost:8222/varz
   ```

2. **Verify NATS URLs**:
   ```toml
   [nats]
   urls = ["nats://server1:4222", "nats://server2:4222"]
   # Verify all servers are running
   ```

3. **Increase connection retry settings**:
   ```toml
   [nats]
   connect_retries = 10
   reconnect_wait_seconds = 5
   ```

4. **Check authentication**:
   ```toml
   [nats]
   # Verify credentials match server configuration
   user_name = "harmonylite"
   user_password = "your-password"
   ```

5. **Test NATS connectivity directly**:
   ```bash
   # Install NATS CLI
   curl -sf https://install.nats.io/install.sh | sh
   
   # Test connection
   nats pub test.subject "hello" --server nats://server:4222
   ```

#### Problem: JetStream Errors

**Symptoms**:
- "Failed to create stream" errors
- "No responders available" errors
- Stream memory or storage errors

**Solutions**:

1. **Check JetStream status**:
   ```bash
   curl http://localhost:8222/jsz
   ```

2. **Verify JetStream is enabled** on NATS server

3. **Check storage limits**:
   ```bash
   # On NATS server
   df -h /path/to/jetstream/storage
   ```

4. **Adjust stream settings**:
   ```toml
   [replication_log]
   max_entries = 1024  # Reduce if storage is limited
   ```

5. **Recreate streams** if corrupted:
   ```bash
   # Using NATS CLI
   nats stream ls --server nats://server:4222
   nats stream rm harmonylite-changes-1 --server nats://server:4222
   # Then restart HarmonyLite
   ```

### Sleep Timeout and Serverless Operation

#### Problem: HarmonyLite Exits Unexpectedly

**Symptoms**:
- Process terminates after a period of inactivity
- Log shows "No more events to process, initiating shutdown"

**Solutions**:

1. **Check sleep timeout setting**:
   ```toml
   # Disable automatic shutdown by setting to 0 (default)
   sleep_timeout = 0
   ```

2. **Adjust timeout duration** if you want the serverless behavior:
   ```toml
   # Set longer timeout in milliseconds, e.g., 30 minutes
   sleep_timeout = 1800000
   ```

3. **Ensure your orchestration system** can handle the expected restarts if using serverless mode

## Fixing Triggers and Schema Issues

### Problem: Missing or Corrupted Triggers

**Symptoms**:
- Changes not being captured
- Missing change log tables
- Schema change errors

**Solutions**:

1. **Check if triggers exist**:
   ```sql
   SELECT name FROM sqlite_master WHERE type='trigger' AND name LIKE '__harmonylite%';
   ```

2. **Clean up and reinstall triggers**:
   ```bash
   harmonylite -config /path/to/config.toml -cleanup
   ```

3. **Verify SQLite version compatibility**:
   ```bash
   sqlite3 --version
   # Should be 3.35.0 or newer
   ```

4. **Enable trusted schema in applications**:
   ```sql
   PRAGMA trusted_schema = ON;
   ```

### Problem: Schema Changes Break Replication

**Symptoms**:
- Errors after changing table structures
- "no such column" errors
- Replication stops after ALTER TABLE operations

**Solutions**:

1. **Proper schema change procedure**:
   - Stop applications
   - Apply changes on one node
   - Run cleanup to reset triggers:
     ```bash
     harmonylite -config /path/to/config.toml -cleanup
     ```
   - Restart HarmonyLite
   - Wait for replication
   - Repeat on other nodes

2. **Verify table structure is identical** on all nodes:
   ```sql
   .schema table_name
   ```

3. **Check for foreign key issues**:
   ```sql
   PRAGMA foreign_key_check;
   ```

## Recovery Procedures

### Full Node Recovery

If a node is completely corrupted or needs to be rebuilt:

1. **Stop HarmonyLite**:
   ```bash
   systemctl stop harmonylite
   ```

2. **Clean up existing files**:
   ```bash
   rm /var/lib/harmonylite/data.db
   rm /var/lib/harmonylite/seq-map.cbor
   ```

3. **Start HarmonyLite** (it will recover automatically):
   ```bash
   systemctl start harmonylite
   ```

4. **Monitor logs** for recovery progress:
   ```bash
   journalctl -u harmonylite -f
   ```

### Manual Database Repair

For advanced recovery when automatic procedures fail:

1. **Create a backup first**:
   ```bash
   cp /var/lib/harmonylite/data.db /var/lib/harmonylite/data.db.bak
   ```

2. **Try SQLite recovery**:
   ```bash
   sqlite3 /var/lib/harmonylite/data.db "PRAGMA integrity_check;"
   ```

3. **Dump and restore** if integrity check fails:
   ```bash
   # Dump schema
   echo .schema | sqlite3 /var/lib/harmonylite/data.db.bak > schema.sql
   
   # Dump data (excluding HarmonyLite tables)
   sqlite3 /var/lib/harmonylite/data.db.bak <<EOF
   .mode insert
   .output data.sql
   SELECT * FROM sqlite_master WHERE type='table' AND name NOT LIKE '__harmonylite%';
   .quit
   EOF
   
   # Create new database
   sqlite3 /var/lib/harmonylite/data.db < schema.sql
   sqlite3 /var/lib/harmonylite/data.db < data.sql
   
   # Reset sequence map
   rm /var/lib/harmonylite/seq-map.cbor
   
   # Restart HarmonyLite
   systemctl restart harmonylite
   ```

## Diagnostic Commands Reference

|Issue|Diagnostic Command|What to Look For|
|---|---|---|
|Node Status|`systemctl status harmonylite`|Active (running) status|
|Logs|`journalctl -u harmonylite -n 100`|Recent error messages|
|Process Resources|`ps -o pid,%cpu,%mem,vsz,rss -p $(pgrep harmonylite)`|CPU/memory usage|
|Health Check|`curl http://localhost:8090/health`|"status":"healthy"|
|NATS Status|`curl http://localhost:8222/varz`|Server running, connections|
|NATS Streams|`curl http://localhost:8222/jsz?streams=1`|Stream existence, message counts|
|Database Size|`du -sh /var/lib/harmonylite/data.db`|Growth trends|
|Database Integrity|`echo "PRAGMA integrity_check;" \| sqlite3 /var/lib/harmonylite/data.db`|"ok" result|
|Triggers|`echo "SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name LIKE '__harmonylite%';" \| sqlite3 /var/lib/harmonylite/data.db`|Non-zero count|
|Change Log Tables|`echo "SELECT count(*) FROM sqlite_master WHERE type='table' AND name LIKE '__harmonylite%';" \| sqlite3 /var/lib/harmonylite/data.db`|Non-zero count|
|Pending Changes|`echo "SELECT count(*) FROM __harmonylite___global_change_log;" \| sqlite3 /var/lib/harmonylite/data.db`|Should be low or zero|
|Network Connectivity|`ss -tpln \| grep harmonylite`|Listening ports|


## Getting More Help

If you're still having issues after following this guide:

1. **Check GitHub Issues** for similar problems and solutions

2. **Gather diagnostic information**:
   ```bash
   # Create diagnostic bundle
   mkdir -p /tmp/harmonylite-diag
   cp /etc/harmonylite/config.toml /tmp/harmonylite-diag/
   journalctl -u harmonylite -n 1000 > /tmp/harmonylite-diag/journal-logs.txt
   curl http://localhost:3010/metrics > /tmp/harmonylite-diag/metrics.txt
   curl http://localhost:8222/varz > /tmp/harmonylite-diag/nats-varz.json
   curl http://localhost:8222/jsz > /tmp/harmonylite-diag/nats-jsz.json
   harmonylite -version > /tmp/harmonylite-diag/version.txt
   tar -czf harmonylite-diag.tar.gz -C /tmp harmonylite-diag
   ```

3. **Open a GitHub Issue** with the diagnostic bundle and detailed description of your problem

4. **Join Community Discussion** for assistance from other users and developers