# Quick Start Guide

This guide walks you through the process of setting up HarmonyLite on your system and creating a basic replication cluster. Follow these steps to get started with distributed SQLite replication in minutes.

## Prerequisites

Before proceeding, ensure your system meets the following requirements:

- **Operating System**: Linux or macOS (Windows users can use WSL)
- **SQLite**: Version 3.35.0 or newer
- **tar**: Required to extract the release archive
- **Internet Access**: To download the latest release

## Installing Dependencies

### On Ubuntu/Debian

```bash
sudo apt update
sudo apt install -y sqlite3 tar
```

### On macOS (using Homebrew)

```bash
brew install sqlite tar
```

### Verifying Installation

Check that SQLite is properly installed:

```bash
sqlite3 --version
```

You should see output like `SQLite 3.x.x`.

## Download and Install HarmonyLite

1. **Download the Latest Release**

   ```bash
   # Replace 'vX.Y.Z' with the latest version from GitHub releases
   curl -L https://github.com/wongfei2009/harmonylite/releases/download/vX.Y.Z/harmonylite-vX.Y.Z-linux-amd64.tar.gz -o harmonylite.tar.gz
   ```

2. **Extract the Archive**

   ```bash
   tar -xzf harmonylite.tar.gz
   cd harmonylite-vX.Y.Z
   ```

3. **Move the Binary to a System Path** (optional)

   ```bash
   sudo mv harmonylite /usr/local/bin/
   ```

## Create a Simple Two-Node Cluster

We'll set up a basic cluster with two nodes replicating a simple database.

### Step 1: Create Configuration Files

First, create configuration files for each node:

**node1-config.toml**:
```toml
# Node 1 Configuration
db_path = "/tmp/harmonylite-1.db"
node_id = 1
seq_map_path = "/tmp/harmonylite-1-seq-map.cbor"

[replication_log]
shards = 1
max_entries = 1024
replicas = 2

[snapshot]
enabled = true
interval = 3600000
store = "nats"
```

**node2-config.toml**:
```toml
# Node 2 Configuration
db_path = "/tmp/harmonylite-2.db"
node_id = 2
seq_map_path = "/tmp/harmonylite-2-seq-map.cbor"

[replication_log]
shards = 1
max_entries = 1024
replicas = 2

[snapshot]
enabled = true
interval = 3600000
store = "nats"
```

### Step 2: Initialize a Test Database

Create a simple test database with a table:

```bash
sqlite3 /tmp/harmonylite-1.db <<EOF
CREATE TABLE notes (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
EOF
```

```bash
sqlite3 /tmp/harmonylite-2.db <<EOF
CREATE TABLE notes (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
EOF
```

### Step 3: Start the First Node

Start the first HarmonyLite node:

```bash
./harmonylite -config node1-config.toml -cluster-addr localhost:4221 -cluster-peers nats://localhost:4222/
```

### Step 4: Start the Second Node

Open a new terminal window and start the second node:

```bash
./harmonylite -config node2-config.toml -cluster-addr localhost:4222 -cluster-peers nats://localhost:4221/
```

### Step 5: Test Replication

1. Open a new terminal window and insert data to the first node:

   ```bash
   sqlite3 /tmp/harmonylite-1.db
   ```

   In the SQLite shell:

   ```sql
   PRAGMA trusted_schema = ON;
   INSERT INTO notes (title, content) VALUES ('First Note', 'This is a test of HarmonyLite replication');
   .exit
   ```

2. Verify replication to the second node:

   ```bash
   sqlite3 /tmp/harmonylite-2.db
   ```

   In the SQLite shell:

   ```sql
   SELECT * FROM notes;
   .exit
   ```

   You should see the record you inserted in the first node.

## Next Steps

Congratulations! You've set up a basic HarmonyLite cluster. From here, you can:

- Try our [complete demo](demo.md) with Pocketbase
- Learn about [architecture and concepts](architecture.md)
- Explore [configuration options](configuration-reference.md)
- Set up a [production deployment](production-deployment.md)

## Troubleshooting

If you encounter issues:

- Check that both nodes are running and connected
- Ensure `PRAGMA trusted_schema = ON` is set before inserting data
- Verify that port 4221 and 4222 are available on your system
- Check log output for error messages
- See the [troubleshooting guide](troubleshooting.md) for more help