# Overview

## What is HarmonyLite?

HarmonyLite is a distributed SQLite replication system designed with a leaderless architecture and eventual consistency. It leverages the fault-tolerant [NATS JetStream](https://nats.io/) to provide robust, multi-directional replication across database nodes without the need for a primary server.

## Why HarmonyLite?

SQLite is the most widely deployed database engine globally, embedded in countless applications across various platforms. HarmonyLite enhances SQLite by offering:

- **Horizontal Scaling**: Ideal for read-heavy applications.
- **Multi-Directional Writes**: Allows writes on all nodes without a central point of control.
- **Eventual Consistency**: Achieves consistency without global locking, ensuring high availability.
- **Lightweight Sidecar**: Runs alongside your existing processes with minimal overhead.
- **No Code Changes**: Integrates seamlessly with your current SQLite-based applications.

## Quick Start Guide

This section walks you through setting up a demonstration HarmonyLite cluster on a Unix-like system (Linux or macOS). We'll cover prerequisites, dependency installation, and detailed steps to get you started.

### Prerequisites

Before proceeding, ensure your system meets the following requirements:

- **Unix-like Environment**: Linux or macOS (Windows users can use WSL - see note below).
- **SQLite**: Required for interacting with HarmonyLite databases via the `sqlite3` command-line tool.
- **tar**: Needed to extract the HarmonyLite release archive.
- **Internet Access**: Necessary to download the latest release from GitHub.

**Note for Windows Users**: HarmonyLite is primarily designed for Unix-like systems. On Windows, you can use the Windows Subsystem for Linux (WSL) to follow this guide. Install WSL, then proceed with a Linux distribution (e.g., Ubuntu) within WSL.

### Installing Dependencies

To set up HarmonyLite, you need to install SQLite and ensure `tar` is available. Here’s how to install these dependencies on common systems:

#### On Ubuntu/Debian

```bash
sudo apt update
sudo apt install -y sqlite3 tar
```

- `sqlite3`: Installs the SQLite command-line tool.
- `tar`: Typically pre-installed, but included to ensure availability.

#### On macOS (using Homebrew)

```bash
brew install sqlite tar
```

- If you don’t have Homebrew, install it first with:
  ```bash
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  ```

#### Verifying Installation

Check that both tools are installed:

```bash
sqlite3 --version
tar --version
```

You should see version outputs (e.g., `SQLite 3.x.x` and `tar (GNU tar) 1.x.x`). If either command fails, revisit the installation steps.

### Steps

Follow these steps to set up and test a HarmonyLite cluster:

1. **Download and Extract the Latest Release**

   Download the latest HarmonyLite release from GitHub and extract it:

   ```bash
   # Replace 'vX.Y.Z' with the actual version number from the latest release
   curl -L https://github.com/wongfei2009/harmonylite/releases/download/vX.Y.Z/harmonylite-vX.Y.Z-linux-amd64.tar.gz -o harmonylite.tar.gz
   tar vxzf harmonylite.tar.gz
   ```

   - This extracts the HarmonyLite binary and example files (e.g., `harmonylite`, `examples/`, `config.toml`).
   - Adjust the URL based on your system architecture (e.g., `linux-amd64`, `darwin-arm64`) and the latest version available at [releases](https://github.com/wongfei2009/harmonylite/releases/latest).

2. **Start a Demonstration Cluster**

   Launch a three-node cluster using the provided script:

   ```bash
   cd examples
   ./run-cluster.sh
   ```

   - **What it does**: This script creates three SQLite databases (`/tmp/harmonylite-1.db`, `/tmp/harmonylite-2.db`, `/tmp/harmonylite-3.db`) with a sample `Books` table, then starts three HarmonyLite nodes with embedded NATS servers for replication.
   - **Output**: You’ll see logs indicating the nodes are running. The script runs in the foreground; open new terminal windows for the next steps.
   - **Stopping the Cluster**: Press `Ctrl+C` in the terminal running the script to stop all nodes.

3. **Insert Data into the First Database**

   Add a sample record to the first node’s database:

   ```bash
   sqlite3 /tmp/harmonylite-1.db
   ```

   In the SQLite shell:

   ```sql
   PRAGMA trusted_schema = ON; -- Enables triggers required by HarmonyLite
   INSERT INTO Books (title, author, publication_year) VALUES ('Pride and Prejudice', 'Jane Austen', 1813);
   .exit
   ```

   - **Explanation**: `PRAGMA trusted_schema = ON` allows HarmonyLite’s triggers to function, which rely on custom SQLite functions. The `INSERT` adds a book record to the `Books` table.

4. **Verify Replication on the Second Database**

   Check if the data replicated to the second node:

   ```bash
   sqlite3 /tmp/harmonylite-2.db
   ```

   In the SQLite shell:

   ```sql
   SELECT * FROM Books WHERE title = 'Pride and Prejudice';
   .exit
   ```

   - **Expected Output**: You should see `8|Pride and Prejudice|Jane Austen|1813` (the ID may differ). This confirms replication from node 1 to node 2.
   - **Timing**: Replication typically occurs within seconds, but allow a brief moment for propagation.

5. **(Optional) Verify Replication on the Third Database**

   For completeness, verify the third node:

   ```bash
   sqlite3 /tmp/harmonylite-3.db
   ```

   In the SQLite shell:

   ```sql
   SELECT * FROM Books WHERE title = 'Pride and Prejudice';
   .exit
   ```

   - **Purpose**: Ensures replication spans all three nodes, demonstrating HarmonyLite’s multi-node synchronization.

### Success!

If you see the inserted record in both the second and third databases, congratulations! Your HarmonyLite cluster is working, and changes are propagating across all nodes.

## How HarmonyLite Works

HarmonyLite employs Change Data Capture (CDC) using SQLite triggers to monitor database changes. Here’s the process:

1. **Change Capture**: SQLite triggers log modifications to internal tracking tables.
2. **Publishing**: Changes are sent to NATS JetStream, a reliable messaging system.
3. **Replication**: All nodes subscribe to these changes and apply them in a consistent order.
4. **Conflict Resolution**: A “last-writer-wins” strategy resolves conflicts, ensuring eventual consistency.

For a deeper dive, see the [Internals documentation](internals.md).

### Comparison with Other Solutions

| Feature               | HarmonyLite       | rqlite           | dqlite           | LiteFS           |
|-----------------------|-------------------|------------------|------------------|------------------|
| **Architecture**      | Leaderless        | Leader-follower  | Leader-follower  | Primary-replica  |
| **Consistency**       | Eventual          | Strong           | Strong           | Strong           |
| **Write Nodes**       | All nodes         | Leader only      | Leader only      | Primary only     |
| **Application Changes** | None            | API changes      | API changes      | VFS layer        |
| **Replication Level** | Logical (row)     | Logical (SQL)    | Physical         | Physical         |

Unlike [rqlite](https://github.com/rqlite/rqlite), [dqlite](https://dqlite.io/), and [LiteFS](https://github.com/superfly/litefs), which rely on a single write node, HarmonyLite offers:

- **Write Anywhere**: Any node can accept writes.
- **No Locking**: Avoids performance bottlenecks from global coordination.
- **Seamless Integration**: No need to modify your application.
- **Sidecar Design**: Runs independently alongside your app.

## FAQ

### How Are Race Conditions Handled?

HarmonyLite maps each row to a specific JetStream based on a hash of the table name and primary keys:

1. Nodes publish changes to the same JetStream stream.
2. NATS JetStream’s [RAFT consensus](https://docs.nats.io/running-a-nats-service/configuration/clustering/jetstream_clustering#raft) orders the changes.
3. All nodes apply changes in the same sequence.
4. The last write wins, ensuring eventual consistency.

**Note**: Transactions across multiple tables lack serializability to avoid locking, prioritizing performance.

### Does Capturing Changes Impact Storage?

Yes, change logs require temporary storage:

- **Duration**: Logs are processed and cleaned up quickly (configurable via `cleanup_interval`).
- **Cost**: Minimal compared to modern storage capacities and replication benefits.

### How Do I Clean Up HarmonyLite Artifacts?

To remove triggers and log tables:

```bash
harmonylite -config /path/to/config.toml -cleanup
```

Replace `/path/to/config.toml` with your configuration file path (e.g., `examples/node-1-config.toml`).

### How Many Shards Should I Configure?

A single shard suffices for most use cases. Increase shards if:

- You experience high write throughput.
- NATS JetStream performance lags.
- Data partitioning is needed.

Adjust `shards` in `config.toml` based on your workload.

### Can I Use HarmonyLite in a Primary-Replica Setup?

Yes, configure it as follows:

- **Replicas**: Set `publish=false` to prevent publishing changes.
- **Primary**: Set `replicate=false` to ignore inbound changes.

Edit `config.toml` for each node accordingly.
