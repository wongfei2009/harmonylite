# HarmonyLite Demo with Pocketbase

This guide demonstrates how to set up a practical application using HarmonyLite for SQLite replication. We'll use **Pocketbase** as our application backend and configure it to work with HarmonyLite's distributed database system.

## Overview

In this demo, you'll:

- Set up a two-node HarmonyLite cluster to replicate the database
- Configure Pocketbase instances with a predefined schema to use these replicated databases
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
mkdir -p harmonylite-demo/{pb-1,pb-2}
cd harmonylite-demo
```

## Step 2: Configure and Start Pocketbase

Now we'll set up two Pocketbase instances, each connected to a separate database managed by HarmonyLite.

Open two new terminal windows and navigate to the demo directory in each:

**Terminal 1 - Pocketbase 1**:
```bash
./pocketbase serve --dir=./pb-1 --http=localhost:8090
```

**Terminal 2 - Pocketbase 2**:
```bash
./pocketbase serve --dir=./pb-2 --http=localhost:8091
```

You can now access each Pocketbase admin dashboard at:
- Node 1: http://localhost:8090/_/
- Node 2: http://localhost:8091/_/

## Step 3: Configure HarmonyLite

We'll create configuration files for two HarmonyLite nodes, each managing a Pocketbase database.

Create the following files:

**node-1-config.toml**:
```toml
db_path = "./pb-1/data.db"
node_id = 1
seq_map_path = "./pb-1/seq-map.cbor"

[replication_log]
shards = 1
max_entries = 1024
replicas = 2
compress = true

[snapshot]
enabled = true
interval = 3600000
store = "nats"
```

**node-2-config.toml**:
```toml
db_path = "./pb-2/data.db"
node_id = 2
seq_map_path = "./pb-2/seq-map.cbor"

[replication_log]
shards = 1
max_entries = 1024
replicas = 2
compress = true

[snapshot]
enabled = true
interval = 3600000
store = "nats"
```

## Step 4: Start HarmonyLite Nodes

Open two separate terminal windows and start each HarmonyLite node:

**Terminal 3 - Node 1**:
```bash
harmonylite -config node-1-config.toml -cluster-addr localhost:4221 -cluster-peers 'nats://localhost:4222/'
```

**Terminal 4 - Node 2**:
```bash
harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/'
```

Verify both nodes are running and connected by checking the log output for successful connection messages.

## Step 5: Set Up the Admin Account


For Pocketbase instance (Node 1):

**Create an Admin Account**:
- Open the admin dashboard (e.g., http://localhost:8090/_/ for Node 1)
- Follow the prompts to create a new admin account

The admin account will be replicated to Node 2 automatically.

## Step 6: Set Up the Demo Application Schema

Since HarmonyLite does not propagate database schemas, we need to manually set up identical schemas in both Pocketbase instances before adding data.

For each Pocketbase instance (Node 1 and Node 2):

**Create the "notes" Collection**:
- In the admin dashboard, click "New collection"
- Name it "notes"
- Add the following fields:
   - `title` (text, required)
   - `content` (text)
   - `is_important` (boolean)
- Click "Create"

Repeat this process for the second instance (http://localhost:8091/_/) to ensure both have the same schema.

## Step 7: Test Replication

Now let's verify that data replication works across both instances:

1. **Add Sample Data in Node 1**:
   - Open http://localhost:8090/_/
   - Navigate to the "notes" collection
   - Click "New record"
   - Fill in a title (e.g., "Test Note") and content (e.g., "This is a test")
   - Click "Save"

2. **Verify Replication in Node 2**:
   - Wait a few seconds for replication to occur
   - Open http://localhost:8091/_/
   - Navigate to the "notes" collection
   - Check that the note from Node 1 appears

3. **Test Bidirectional Replication**:
   - In Node 2 (http://localhost:8091/_/), create a new note
   - Verify it appears in Node 1 (http://localhost:8090/_/)

## Step 8: Test Fault Tolerance

Let's simulate a node failure to see how the system handles it:

1. Stop Node 2 (press Ctrl+C in Terminal 2)

2. Create a new note in Node 1 via http://localhost:8090/_/

3. Verify that Node 1 remains operational and the note is saved locally

4. Restart Node 2:
   ```bash
   harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/'
   ```

5. Check Node 2’s "notes" collection (http://localhost:8091/_/) to confirm it catches up and receives the changes made while it was offline

## Real-World Application Ideas

Now that you have a working HarmonyLite + Pocketbase setup, here are some real-world applications you could build:

- **Distributed Task Manager**: Create a task management app with team collaboration
- **Content Management System**: Build a CMS with multiple editing nodes
- **Inventory System**: Track inventory across multiple locations with local databases
- **Field Service App**: Enable offline-first data collection that syncs when connectivity is restored
- **Edge Computing Dashboard**: Collect data at edge locations with eventual central reporting

## Conclusion

You've successfully set up a distributed application using HarmonyLite and Pocketbase with two nodes. This demonstrates how HarmonyLite enables:

1. **High Availability**: The system continues to function even when nodes go offline
2. **Horizontal Scaling**: You can add more nodes to handle increased load
3. **Eventual Consistency**: Changes propagate to all nodes over time
4. **Zero Application Changes**: Pocketbase works without modifications

## Next Steps

- Explore [advanced configuration options](configuration-reference.md)
- Learn about [production deployment](production-deployment.md)
- Understand the [architecture and concepts](architecture.md) in depth
