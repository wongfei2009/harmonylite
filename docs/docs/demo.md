# HarmonyLite Demo with Pocketbase

This guide demonstrates how to set up a practical application using HarmonyLite for SQLite replication. We'll use **Pocketbase** as our application backend and configure it to work with HarmonyLite's distributed database system.

## Overview

In this demo, you'll:

- Set up a three-node HarmonyLite cluster to replicate the database
- Configure Pocketbase instances to use these replicated databases
- Create a distributed note-taking application with replication
- Test fault tolerance and recovery

By the end, you'll have a practical understanding of how HarmonyLite works in a real-world scenario.

## What You'll Need

- **HarmonyLite**: Downloaded and available on your system
- **Pocketbase**: Downloaded from [Pocketbase.io](https://pocketbase.io/docs/)
- A terminal and basic command-line knowledge
- About 15-20 minutes to complete the setup

## Step 1: Set Up Directory Structure

Create a directory for our demo and necessary subdirectories:

```bash
mkdir -p harmonylite-demo/{pb-1,pb-2,pb-3}
cd harmonylite-demo
```

## Step 2: Configure HarmonyLite

We'll create configuration files for three HarmonyLite nodes, each managing a Pocketbase database.

Create the following files:

**node-1-config.toml**:
```toml
database_path = "./pb-1/data.db"
node_id = 1
seq_map_path = "./pb-1/seq-map.cbor"

[replication_log]
shards = 1
max_entries = 1024
replicas = 3
compress = true

[snapshot]
enabled = true
interval = 3600000
store = "nats"
```

**node-2-config.toml**:
```toml
database_path = "./pb-2/data.db"
node_id = 2
seq_map_path = "./pb-2/seq-map.cbor"

[replication_log]
shards = 1
max_entries = 1024
replicas = 3
compress = true

[snapshot]
enabled = true
interval = 3600000
store = "nats"
```

**node-3-config.toml**:
```toml
database_path = "./pb-3/data.db"
node_id = 3
seq_map_path = "./pb-3/seq-map.cbor"

[replication_log]
shards = 1
max_entries = 1024
replicas = 3
compress = true

[snapshot]
enabled = true
interval = 3600000
store = "nats"
```

## Step 3: Start HarmonyLite Nodes

Open three separate terminal windows and start each HarmonyLite node:

**Terminal 1 - Node 1**:
```bash
harmonylite -config node-1-config.toml -cluster-addr localhost:4221
```

**Terminal 2 - Node 2**:
```bash
harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/'
```

**Terminal 3 - Node 3**:
```bash
harmonylite -config node-3-config.toml -cluster-addr localhost:4223 -cluster-peers 'nats://localhost:4221/,nats://localhost:4222/'
```

Verify all nodes are running and connected by checking the log output for successful connection messages.

## Step 4: Configure and Start Pocketbase

Now we'll set up three Pocketbase instances, each connected to a separate database managed by HarmonyLite.

Open three new terminal windows and navigate to the demo directory in each:

**Terminal 4 - Pocketbase 1**:
```bash
cd harmonylite-demo
./pocketbase serve --dir=./pb-1 --http=localhost:8090
```

**Terminal 5 - Pocketbase 2**:
```bash
cd harmonylite-demo
./pocketbase serve --dir=./pb-2 --http=localhost:8091
```

**Terminal 6 - Pocketbase 3**:
```bash
cd harmonylite-demo
./pocketbase serve --dir=./pb-3 --http=localhost:8092
```

You can now access each Pocketbase admin dashboard at:
- Node 1: http://localhost:8090/_/
- Node 2: http://localhost:8091/_/
- Node 3: http://localhost:8092/_/

## Step 5: Create a Demo Application

Let's set up our note-taking application through the Pocketbase Admin UI:

1. **Create an Admin Account**:
   - Open http://localhost:8090/_/ in your browser
   - Follow the prompts to create a new admin account

2. **Create a Collection**:
   - In the admin dashboard, click "New collection"
   - Name it "notes"
   - Add the following fields:
     - title (text, required)
     - content (text)
     - is_important (boolean)
   - Click "Create"

3. **Add Sample Data**:
   - Navigate to the "notes" collection
   - Click "New record"
   - Fill in a title and content
   - Click "Save"

## Step 6: Test Replication

Now let's verify that our data is being replicated across all instances:

1. Wait a few seconds for replication to occur

2. Open each Pocketbase admin dashboard and navigate to the "notes" collection:
   - http://localhost:8091/_/ (Node 2)
   - http://localhost:8092/_/ (Node 3)

3. You should see the same notes in all three instances

4. Try creating a new note in one of the other instances and verify it appears in all three

## Step 7: Test Fault Tolerance

Let's simulate a node failure to see how the system handles it:

1. Stop one of the HarmonyLite nodes (e.g., press Ctrl+C in Terminal 2)

2. Create a new note in one of the remaining nodes

3. The note should still be replicated to the other active node

4. Now restart the stopped node:
   ```bash
   harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/'
   ```

5. Verify that the node catches up and receives all the changes that occurred while it was offline

## Real-World Application Ideas

Now that you have a working HarmonyLite + Pocketbase setup, here are some real-world applications you could build:

- **Distributed Task Manager**: Create a task management app with team collaboration
- **Content Management System**: Build a CMS with multiple editing nodes
- **Inventory System**: Track inventory across multiple locations with local databases
- **Field Service App**: Enable offline-first data collection that syncs when connectivity is restored
- **Edge Computing Dashboard**: Collect data at edge locations with eventual central reporting

## Conclusion

You've successfully set up a distributed application using HarmonyLite and Pocketbase. This demonstrates how HarmonyLite enables:

1. **High Availability**: The system continues to function even when nodes go offline
2. **Horizontal Scaling**: You can add more nodes to handle increased load
3. **Eventual Consistency**: Changes propagate to all nodes over time
4. **Zero Application Changes**: Pocketbase works without modifications

## Next Steps

- Explore [advanced configuration options](configuration-reference.md)
- Learn about [production deployment](production-deployment.md)
- Understand the [architecture and concepts](architecture.md) in depth