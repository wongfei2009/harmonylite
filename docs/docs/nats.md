# NATS

## What is NATS?

[NATS](https://nats.io/) is a high-performance, lightweight, open-source messaging system designed for distributed systems, cloud-native applications, and IoT. It connects services and applications using a publish-subscribe model, offering simplicity, security, and scalability. Known for its minimal resource footprint, NATS handles high-throughput messaging with low latency, making it a perfect fit for real-time data exchange.

In HarmonyLite, NATS is the backbone of replication, enabling leaderless, eventually consistent synchronization across SQLite databases. It powers the communication layer, allowing nodes to share change logs and snapshots without a central coordinator, ensuring a decentralized and robust system.

## Why NATS in HarmonyLite?

HarmonyLite chooses NATS for these key advantages:

- **Leaderless Architecture**: NATS’s decentralized model lets any node publish or subscribe, matching HarmonyLite’s leaderless replication approach.
- **Eventual Consistency**: Through NATS JetStream, HarmonyLite propagates changes asynchronously, ensuring nodes eventually align.
- **Scalability**: Adding nodes is seamless with NATS, supporting horizontal scaling without complexity.
- **Flexible Deployment**: Options include an embedded NATS server for simplicity or external servers for advanced setups.
- **Fault Tolerance**: JetStream’s replication keeps change logs available, even during temporary node outages.

## How HarmonyLite Uses NATS

HarmonyLite integrates NATS in two main ways:

### 1. Change Log Replication
- **Purpose**: Publishes and subscribes to database change logs (e.g., `INSERT`, `UPDATE`, `DELETE`) via NATS JetStream.
- **How It Works**: Changes are wrapped in a `ChangeLogEvent` (`db/change_log_event.go`) and sent to subjects prefixed with `harmonylite-change-log` (configurable in `config.toml`). The `Replicator` (`logstream/replicator.go`) manages publishing to shards and consuming updates from other nodes.
- **Sharding**: Controlled by `replication_log.shards`, this splits logs across multiple streams (e.g., `harmonylite-changes-1`), boosting performance.

### 2. Snapshot Storage
- **Purpose**: Stores and retrieves database snapshots using NATS Object Store when `snapshot.store = "nats"`.
- **How It Works**: The `NatsStorage` component (`snapshot/nats_storage.go`) handles uploads and downloads, enabling fast recovery from snapshots.

### Embedded vs. External NATS

HarmonyLite offers two deployment modes:

- **Embedded NATS Server**:
  - **Trigger**: Activates when `nats.urls` is empty in `config.toml` (`cfg/config.go`).
  - **Setup**: Each node runs its own NATS server at `nats.bind_address` (default: `0.0.0.0:4222`), clustering via `-cluster-peers`. Nodes are named `harmonylite-node-<node_id>`.
  - **Use Case**: Great for development, testing, or small setups without separate NATS infrastructure.

- **External NATS Server**:
  - **Trigger**: Used when `nats.urls` lists server addresses (e.g., `["nats://localhost:4222"]`).
  - **Setup**: HarmonyLite connects as a client to an existing NATS cluster.
  - **Use Case**: Ideal for production with a dedicated, highly available NATS setup.

Switching modes is as simple as editing `nats.urls` in `config.toml`—no code changes needed.

## Configuring NATS in HarmonyLite

NATS settings are managed via the `[nats]` section in `config.toml` and command-line flags. Below, we cover standard options and add a guide to modify the default storage location.

### Configuration Options in `config.toml`

- **`urls`**:
  - **Description**: List of NATS server URLs (e.g., `["nats://localhost:4222"]`).
  - **Default**: Empty (`[]`), triggering the embedded server.
  - **Notes**: Supports authentication (e.g., `nats://user:password@host:port`).

- **`subject_prefix`**:
  - **Description**: Prefix for change log subjects.
  - **Default**: `harmonylite-change-log`.
  - **Notes**: Appended with shard numbers for sharded setups.

- **`stream_prefix`**:
  - **Description**: Prefix for JetStream streams.
  - **Default**: `harmonylite-changes`.
  - **Notes**: Names streams in JetStream.

- **`bind_address`**:
  - **Description**: Address and port for the embedded NATS server.
  - **Default**: `0.0.0.0:4222`.
  - **Notes**: Only applies to embedded mode.

- **`server_config`**:
  - **Description**: Path to a custom NATS server config file.
  - **Default**: Empty (uses defaults).
  - **Notes**: Optional, for advanced embedded server tweaks.

- **`connect_retries`**:
  - **Description**: Connection attempts to external servers.
  - **Default**: 5.
  - **Notes**: Only for external mode.

- **`reconnect_wait_seconds`**:
  - **Description**: Delay between reconnect attempts.
  - **Default**: 2.
  - **Notes**: Only for external mode.

### Command-Line Flags

- **`-cluster-addr`**:
  - Defines the embedded server’s cluster listener address (e.g., `127.0.0.1:4222`).
  - Overrides `nats.bind_address` for clustering.
  - Example: `-cluster-addr=127.0.0.1:4222`.

- **`-cluster-peers`**:
  - Lists peer URLs for clustering (e.g., `nats://127.0.0.1:4221,nats://127.0.0.1:4223`).
  - Essential for embedded NATS clusters.
  - Example: `-cluster-peers=nats://127.0.0.1:4221/,nats://127.0.0.1:4223/`.

### Modifying the Default Storage Location

By default, NATS JetStream writes data (change logs and snapshots) to `/tmp` on each node’s filesystem in embedded mode, often in a subdirectory like `/tmp/nats/jetstream`. This is fine for testing, but for production, you might want a more persistent or performance-optimized location (e.g., `/var/lib/nats`). Here’s how to change it:

#### Why Modify It?
- **Persistence**: `/tmp` may be cleared on reboot, risking data loss.
- **Performance**: A faster disk (e.g., SSD) can improve JetStream operations.
- **Organization**: A dedicated directory simplifies monitoring and backups.

#### Steps to Change the Storage Location

1. **Create a NATS Config File**:
   - Create a file (e.g., `nats-server.conf`) to define the JetStream storage directory.
   - Example content:
     ```plaintext
     # Basic server settings
     listen: 0.0.0.0:4222
     
     # JetStream configuration
     jetstream {
       store_dir: "/var/lib/nats/jetstream"
     }
     ```
   - Here, `store_dir` sets the new location. Replace `/var/lib/nats/jetstream` with your desired path (e.g., `/data/nats`).

2. **Ensure Directory Exists and Has Permissions**:
   - On each node, create the directory and set ownership:
     ```bash
     sudo mkdir -p /var/lib/nats/jetstream
     sudo chown $USER:$USER /var/lib/nats/jetstream
     ```
   - Replace `$USER` with the user running HarmonyLite (e.g., `harmonylite` or your username).

3. **Update `config.toml`**:
   - Point HarmonyLite to your config file by adding `server_config`:
     ```toml
     [nats]
     bind_address = "0.0.0.0:4222"
     subject_prefix = "harmonylite-change-log"
     stream_prefix = "harmonylite-changes"
     server_config = "/path/to/nats-server.conf"
     ```
   - Replace `/path/to/nats-server.conf` with the actual file path (e.g., `/etc/harmonylite/nats-server.conf`).

4. **Run HarmonyLite**:
   - Start HarmonyLite with the updated config:
     ```bash
     ./harmonylite -config config.toml -cluster-addr 127.0.0.1:4222 -cluster-peers nats://127.0.0.1:4221/,nats://127.0.0.1:4223/
     ```
   - NATS will now write data to `/var/lib/nats/jetstream` instead of `/tmp`.

5. **Verify the Change**:
   - Check the directory for JetStream files (e.g., `.js` files for streams):
     ```bash
     ls /var/lib/nats/jetstream
     ```
   - Look at HarmonyLite logs (`verbose = true` in `[logging]`) for confirmation of the storage path.

#### Tips
- **Clustering**: Repeat this process on all nodes, ensuring `store_dir` is consistent or uniquely managed per node.
- **External NATS**: For external servers, modify the NATS server’s config directly (not via HarmonyLite), as HarmonyLite only connects as a client.
- **Safety**: Use a persistent disk (not tmpfs) and monitor disk space to avoid running out.

This customization ensures your data is stored where you want it, tailored to your environment’s needs.

### Example Configurations

#### Embedded NATS Cluster (Default Storage)
```toml
[nats]
bind_address = "0.0.0.0:4222"
subject_prefix = "harmonylite-change-log"
stream_prefix = "harmonylite-changes"
```
Run: `./harmonylite -config config.toml -cluster-addr 127.0.0.1:4222 -cluster-peers nats://127.0.0.1:4221/,nats://127.0.0.1:4223/`

#### Embedded NATS with Custom Storage
```toml
[nats]
bind_address = "0.0.0.0:4222"
subject_prefix = "harmonylite-change-log"
stream_prefix = "harmonylite-changes"
server_config = "/etc/harmonylite/nats-server.conf"
```
With `nats-server.conf`:
```plaintext
listen: 0.0.0.0:4222
jetstream {
  store_dir: "/var/lib/nats/jetstream"
}
```
Run: `./harmonylite -config config.toml -cluster-addr 127.0.0.1:4222 -cluster-peers nats://127.0.0.1:4221/,nats://127.0.0.1:4223/`

#### External NATS Server
```toml
[nats]
urls = ["nats://localhost:4222"]
connect_retries = 5
reconnect_wait_seconds = 2
subject_prefix = "harmonylite-change-log"
stream_prefix = "harmonylite-changes"
```
Run: `./harmonylite -config config.toml`

## NATS and Snapshots

For NATS-based snapshot storage (`snapshot.store = "nats"`):

- **`replicas`**:
  - Number of snapshot replicas (default: 1, max: 5).
- **`bucket`**:
  - Bucket name (optional).

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
  - **Embedded**: Check `-cluster-addr` and `-cluster-peers` consistency and port availability.
  - **External**: Ensure `nats.urls` targets a JetStream-enabled server.
- **Embedded Server Fails**:
  - Verify `nats.bind_address` or `-cluster-addr` isn’t in use.
- **Snapshot Errors**:
  - Confirm JetStream is active and `[snapshot.nats]` is correct.
- **Storage Issues**:
  - Check the storage directory (`/tmp` or custom) for permissions and space.

Set `verbose = true` in `[logging]` for detailed logs.

## Learn More About NATS

Dive into the [NATS documentation](https://docs.nats.io/):
- [JetStream](https://docs.nats.io/nats-concepts/jetstream) for streams.
- [Object Store](https://docs.nats.io/nats-concepts/jetstream/obj_store) for snapshots.
- [Clustering](https://docs.nats.io/running-a-nats-service/configuration/clustering) for multi-node setups.

HarmonyLite’s NATS integration offers a flexible, robust foundation for distributed SQLite replication.