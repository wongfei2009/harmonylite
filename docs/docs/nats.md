# NATS

## What is NATS?

[NATS](https://nats.io/) is a high-performance, lightweight, and open-source messaging system designed for distributed systems, cloud-native applications, and IoT. It provides a simple, secure, and scalable way to connect services and applications through a publish-subscribe model. NATS is known for its minimal resource footprint and ability to handle high-throughput messaging with low latency.

In HarmonyLite, NATS serves as the backbone for replication, enabling leaderless, eventually consistent data synchronization across SQLite databases. It powers the communication layer that allows nodes to share change logs and snapshots without requiring a central coordinator.

## Why NATS in HarmonyLite?

HarmonyLite leverages NATS for several key reasons:

- **Leaderless Architecture**: NATS supports a decentralized model where any node can publish or subscribe to messages, aligning with HarmonyLite's leaderless replication design.
- **Eventual Consistency**: Using NATS JetStream, HarmonyLite ensures that database changes are propagated asynchronously, achieving eventual consistency across nodes.
- **Scalability**: NATS allows HarmonyLite to scale horizontally by adding more nodes without complex reconfiguration.
- **Flexible Deployment**: HarmonyLite supports both an embedded NATS server for simple setups and external NATS servers for advanced deployments.
- **Fault Tolerance**: With features like JetStream replication, NATS ensures that messages (change logs) are persisted and available even if nodes go offline temporarily.

## How HarmonyLite Uses NATS

HarmonyLite integrates NATS in two primary ways:

1. **Change Log Replication**:
   - HarmonyLite uses NATS JetStream to publish and subscribe to database change logs (e.g., `INSERT`, `UPDATE`, `DELETE` events).
   - Each change is encapsulated in a `ChangeLogEvent` (see `db/change_log_event.go`) and sent to a NATS subject prefixed with `harmonylite-change-log` (configurable via `nats.subject_prefix` in `config.toml`).
   - The `Replicator` (`logstream/replicator.go`) handles publishing these events to specific shards and listening for incoming changes from other nodes.
   - Sharding is supported via `replication_log.shards` in the configuration, distributing logs across multiple JetStream streams.

2. **Snapshot Storage**:
   - When configured with `snapshot.store = "nats"` in `config.toml`, HarmonyLite uses NATS Object Store (via JetStream) to save and restore database snapshots.
   - The `NatsStorage` component (`snapshot/nats_storage.go`) manages uploading and downloading snapshots, ensuring nodes can recover quickly from a cold start.

### Embedded vs External NATS

HarmonyLite provides flexibility in how NATS is deployed:

- **Embedded NATS Server**:
  - Activated when `nats.urls` is empty in `config.toml` (see `cfg/config.go`).
  - Each HarmonyLite node starts its own NATS server, binding to `nats.bind_address` (default: `0.0.0.0:4222`).
  - Forms a cluster using the `-cluster-peers` flag or runtime configuration, with node names like `harmonylite-node-<node_id>` (see `cfg/config.go`).
  - Ideal for development, testing, or small deployments where a separate NATS infrastructure isn’t desired.

- **External NATS Server**:
  - Used when `nats.urls` specifies one or more NATS server addresses (e.g., `["nats://localhost:4222"]`).
  - HarmonyLite connects as a client to the external NATS cluster, leveraging its existing infrastructure.
  - Suitable for production environments where a dedicated, highly available NATS cluster is already in place.

The choice between embedded and external NATS is determined at startup based on the `nats.urls` configuration, making it easy to switch between modes without code changes.

## Configuring NATS in HarmonyLite

NATS configuration is managed through the `[nats]` section of `config.toml` and command-line flags:

### Configuration Options in `config.toml`

- **`urls`**:
  - List of NATS server URLs (e.g., `["nats://localhost:4222"]`).
  - Leave empty (`[]`) to enable the embedded NATS server.
  - Supports authentication via URL parameters (e.g., `nats://user:password@host:port`).

- **`subject_prefix`**:
  - Prefix for change log subjects (default: `harmonylite-change-log`).
  - Combined with shard numbers for sharded replication.

- **`stream_prefix`**:
  - Prefix for JetStream streams (default: `harmonylite-changes`).
  - Used to name streams in NATS JetStream.

- **`bind_address`**:
  - Address and port for the embedded NATS server (default: `0.0.0.0:4222`).
  - Only applies when using the embedded server.

- **`server_config`**:
  - Path to a custom NATS server configuration file for the embedded server (optional).

- **`connect_retries`**:
  - Number of connection attempts to external NATS servers (default: 5).
  - Only applies when `urls` is non-empty.

- **`reconnect_wait_seconds`**:
  - Time between reconnect attempts to external NATS servers (default: 2).
  - Only applies when `urls` is non-empty.

### Command-Line Flags

- **`-cluster-addr`**:
  - Specifies the binding address for the embedded NATS server’s cluster listener (e.g., `127.0.0.1:4222`).
  - Overrides `nats.bind_address` for clustering purposes.
  - Used to define where this node listens for cluster connections from peers.
  - Example: `-cluster-addr=127.0.0.1:4222`.

- **`-cluster-peers`**:
  - Comma-separated list of NATS peer URLs for clustering (e.g., `nats://127.0.0.1:4221,nats://127.0.0.1:4223`).
  - Required for embedded NATS to form a cluster with other nodes.
  - Example: `-cluster-peers=nats://127.0.0.1:4221/,nats://127.0.0.1:4223/`.

### Example Configurations

#### Embedded NATS Cluster
```toml
[nats]
bind_address = "0.0.0.0:4222"
subject_prefix = "harmonylite-change-log"
stream_prefix = "harmonylite-changes"
```
Run with:
```bash
./harmonylite -config config.toml -cluster-addr 127.0.0.1:4222 -cluster-peers nats://127.0.0.1:4221/,nats://127.0.0.1:4223/
```

#### External NATS Server
```toml
[nats]
urls = ["nats://localhost:4222"]
connect_retries = 5
reconnect_wait_seconds = 2
subject_prefix = "harmonylite-change-log"
stream_prefix = "harmonylite-changes"
```
Run with:
```bash
./harmonylite -config config.toml
```

## NATS and Snapshots

When using NATS for snapshot storage (`snapshot.store = "nats"`), configure the `[snapshot.nats]` section:

- **`replicas`**:
  - Number of replicas for the snapshot object store (default: 1, max: 5).
- **`bucket`**:
  - Name of the bucket for storing snapshots (optional, defaults to a generated name).

Example:
```toml
[snapshot]
enabled = true
store = "nats"

[snapshot.nats]
replicas = 2
bucket = "harmonylite-snapshots"
```

## Troubleshooting NATS Issues

- **Node Not Replicating**:
  - For embedded NATS: Ensure `-cluster-addr` and `-cluster-peers` match across nodes and ports are open.
  - For external NATS: Verify `nats.urls` points to a running NATS server with JetStream enabled.
- **Embedded Server Fails to Start**:
  - Check if `nats.bind_address` or `-cluster-addr` is already in use (e.g., port conflict).
- **Snapshot Errors**:
  - Confirm JetStream is enabled on the NATS server and `[snapshot.nats]` settings are valid.
- **Connection Issues**:
  - Increase `connect_retries` or `reconnect_wait_seconds` for unstable networks when using external NATS.

Logs (controlled by `[logging]` in `config.toml`) provide detailed output for debugging NATS-related issues. Set `verbose = true` for more granularity.

## Learn More About NATS

For deeper insights, explore the [official NATS documentation](https://docs.nats.io/). Key topics relevant to HarmonyLite:
- [JetStream](https://docs.nats.io/nats-concepts/jetstream) for persistent streams.
- [Object Store](https://docs.nats.io/nats-concepts/jetstream/obj_store) for snapshot storage.
- [Clustering](https://docs.nats.io/running-a-nats-service/configuration/clustering) for multi-node setups.

HarmonyLite’s flexible integration with NATS—whether embedded or external—makes it a powerful tool for distributed SQLite replication, balancing simplicity and robustness.
