# HarmonyLite Demo Examples

This directory contains scripts for demonstrating HarmonyLite's database replication capabilities using different applications and scenarios.

## Available Demos

### 1. Basic Cluster Demo

The `run-cluster.sh` script demonstrates a basic three-node HarmonyLite cluster with a simple Books database.

```bash
./run-cluster.sh
```

This script:
- Sets up a three-node HarmonyLite cluster
- Creates a sample Books database on each node
- Demonstrates basic replication functionality

### 2. Schema Migration Test

The `run-schema-migration-test.sh` script tests HarmonyLite's schema versioning and migration capabilities, including automatic detection of schema mismatches and cluster-wide consistency.

```bash
./run-schema-migration-test.sh
```

This automated test verifies:
- **Schema Hash Computation**: Deterministic SHA-256 hashing of database schemas
- **Schema Registry**: Nodes publish schema state to NATS KV cluster
- **Mismatch Detection**: Different schema hashes are detected across nodes
- **Rolling Upgrade Workflow**: Stop node, apply DDL, restart node
- **Schema Convergence**: All nodes converge to same hash after upgrade

#### What the Schema Test Does

1. **Initial Setup**: Creates a 3-node cluster with identical Books table schemas
2. **Consistency Check**: Verifies all nodes compute the same schema hash
3. **Simulated Mismatch**: Adds an `email` column to Node 1 only
4. **Registry Verification**: Confirms all nodes publish to cluster registry
5. **Rolling Upgrade**: Applies DDL to remaining nodes
6. **Convergence Check**: Verifies all nodes have matching hashes after upgrade

#### Schema Status Commands

After running the test, you can check schema status:

```bash
# Local node schema status
./harmonylite -schema-status

# Cluster-wide schema status (shows all nodes)
./harmonylite -schema-status-cluster
```

Example output:
```
Schema Status for Node 1
  Schema Hash: 6bae09c0... (first 8 chars)
  Node ID: 1
  HarmonyLite Version: v0.10.0
  Watched Tables: Books

Cluster Schema Status (3 nodes)
  Status: Consistent
  All nodes have schema: 6bae09c0...
```

#### Use Cases

This test is valuable for:
- **CI/CD Pipelines**: Automated verification of schema versioning
- **Development**: Testing schema migration workflows
- **Troubleshooting**: Diagnosing schema mismatch issues

### 3. PocketBase Demo

The `run-pocketbase-demo.sh` script automates the setup and execution of a distributed note-taking application using HarmonyLite for SQLite replication and PocketBase as the backend.

```bash
./run-pocketbase-demo.sh
```

This demo:
- Sets up a two-node HarmonyLite cluster
- Configures two PocketBase instances with:
  - Pre-created admin user
  - "notes" collection schema
- Demonstrates bidirectional replication between nodes
- Shows fault tolerance capabilities

#### PocketBase Demo Options

- `--help`: Display usage information
- `--keep-files`: Don't delete temporary files on exit
- `--pb-path PATH`: Specify an existing PocketBase binary
- `--no-download`: Don't download PocketBase if not found

#### Accessing the PocketBase Demo

When the demo is running, you can access:

- PocketBase Node 1: [http://localhost:8090/_/](http://localhost:8090/_/)
- PocketBase Node 2: [http://localhost:8091/_/](http://localhost:8091/_/)

Login with:
- Email: `admin@example.com`
- Password: `1234567890`

## Customizing Demos

You can modify the configuration files (e.g., `node-X-config.toml`) to explore different HarmonyLite settings or database schemas.

## Schema Versioning Features

HarmonyLite includes comprehensive schema versioning to ensure safe database replication during schema changes and rolling upgrades.

### Key Features

1. **Automatic Schema Tracking**: Every node computes a deterministic SHA-256 hash of its schema
2. **Schema Validation**: Events carry schema metadata; mismatches are detected before replication
3. **Automatic Pause**: Replication pauses when schemas diverge (prevents data corruption)
4. **Self-Healing**: Periodic recompute (every 5 minutes) automatically resumes when schemas match
5. **Cluster Visibility**: NATS KV registry shows schema state across all nodes
6. **Zero Downtime**: Rolling upgrades work without restarts

### Schema Status CLI Commands

```bash
# Check local node schema
./harmonylite -schema-status

# Check cluster-wide schema consistency
./harmonylite -schema-status-cluster
```

### Rolling Schema Upgrade Workflow

1. **Verify Initial State**: Run `-schema-status-cluster` to ensure consistency
2. **Apply DDL to Node 1**: Execute `ALTER TABLE` or other DDL statements
3. **Verify Pause**: Check logs/metrics confirm replication paused on other nodes
4. **Apply DDL to Node 2**: Repeat DDL on second node
5. **Apply DDL to Node 3**: Complete rollout to final node
6. **Verify Resume**: Within 5 minutes, replication automatically resumes
7. **Confirm Sync**: Data queued during mismatch replicates after resume

### Observability

**Metrics:**
- `harmonylite_schema_mismatch_paused{node_id}`: 1 if paused due to mismatch, 0 if running

**Logs:**
- Schema hash computation on startup
- Mismatch detection with hash comparison
- Automatic pause/resume events
- Schema state publishing to cluster registry

### Best Practices

- **Always check cluster status** before starting schema changes
- **Use the schema demo script** to practice rollout procedures
- **Monitor the pause metric** during production upgrades
- **Allow 5-10 minutes** between node upgrades for self-healing
- **Verify data sync** after completing cluster-wide DDL

For detailed design documentation, see `docs/docs/design/schema-versioning.md`.

## Troubleshooting

If you encounter issues:

### General Issues
- Ensure required ports are available
- Check that the HarmonyLite binary is in your PATH or in the current directory
- Examine error messages in the terminal output
- See the [troubleshooting guide](../docs/docs/troubleshooting.md) for more help

### Schema Versioning Issues

**Replication is paused:**
- Check `-schema-status-cluster` to identify which nodes have mismatched schemas
- Verify DDL was applied correctly: `sqlite3 /path/to/db.db ".schema table_name"`
- Wait 5 minutes for self-healing recompute, or restart the node
- Check logs for "schema mismatch detected" messages

**Cluster schema shows inconsistent:**
- Ensure all nodes applied the same DDL statements
- Check for typos in ALTER TABLE commands (case sensitivity matters)
- Verify node is running: stale entries expire after 5 minutes

**Schema status command fails:**
- Ensure NATS cluster is running and accessible
- Check config file has correct cluster peers
- Verify network connectivity between nodes

**Demo script fails:**
- Ensure no other processes are using ports 4221-4223
- Clean up: `rm -rf /tmp/harmonylite-* /tmp/nats*`
- Check harmonylite binary exists: `ls -l harmonylite`
- Review logs at `/tmp/harmonylite-node{1,2,3}.log`
