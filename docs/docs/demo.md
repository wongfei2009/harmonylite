# HarmonyLite Demo with PocketBase

This guide demonstrates setting up a distributed note-taking application using HarmonyLite for SQLite replication and **PocketBase** as the backend, with preconfigured admin and schema.

## Overview

In this demo, you’ll:
- Set up a two-node HarmonyLite cluster for database replication
- Launch PocketBase instances preconfigured with an admin user and "notes" collection
- Test replication and fault tolerance in a distributed note-taking app

By the end, you'll see how HarmonyLite and PocketBase work together seamlessly.

## What You'll Need

- **HarmonyLite**: Downloaded and available
- **PocketBase**: Downloaded from [PocketBase.io](https://pocketbase.io/docs/)
- A terminal and basic command-line skills
- About 10-15 minutes

## Step 1: Set Up Directory Structure

Create the demo directory and subdirectories:

```bash
mkdir -p harmonylite-demo/{pb-1,pb-2,pb-1/pb_migrations}
cd harmonylite-demo
```

## Step 2: Preconfigure PocketBase

We'll initialize PocketBase with a default admin user and "notes" collection using a migration script.

1. **Create Admin User for Node 1**:
   - In the `harmonylite-demo` directory, run:
     ```bash
     ./pocketbase superuser create admin@example.com 1234567890 --dir=./pb-1
     ```

2. **Define "notes" Collection**:
   - Create a migration file in `user_pb_migrations` (e.g., `init_notes.js`):
     ```javascript
     migrate((app) => {
      // Create a new "notes" base collection
      const collection = new Collection({
         name: "notes",
         type: "base",
         listRule: "",
         viewRule: "",
         createRule: "",
         updateRule: "",
         deleteRule: "",
         fields: [
            { name: "title", type: "text", required: true },
            { name: "content", type: "text" },
            { name: "is_important", type: "bool" }
         ]
      });
      
      return app.save(collection);
      }, (app) => {
      // Down migration code (optional - for reverting the changes)
      const collection = app.findCollectionByNameOrId("notes");
      return app.delete(collection);
      });
     ```

3. **Apply Migrations**:
   - For Node 1:
     ```bash
     ./pocketbase migrate --dir=./pb-1 --migrationsDir ./user_pb_migrations
     ```
4. Copy pb-1 to pb-2:
   ```bash
   cp -r pb-1 pb-2
   ```

## Step 3: Start PocketBase Instances

Launch both PocketBase instances with the preconfigured setup:

**Terminal 1 - PocketBase 1**:
```bash
./pocketbase serve --dir=./pb-1 --http=localhost:8090
```

**Terminal 2 - PocketBase 2**:
```bash
./pocketbase serve --dir=./pb-2 --http=localhost:8091
```

Access the admin dashboards:
- Node 1: http://localhost:8090/_/ (login: `admin@example.com`, `1234567890`)
- Node 2: http://localhost:8091/_/ (same credentials)

## Step 4: Configure HarmonyLite

Create configuration files for two HarmonyLite nodes:

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

## Step 5: Start HarmonyLite Nodes

Run each node in a separate terminal:

**Terminal 3 - Node 1**:
```bash
harmonylite -config node-1-config.toml -cluster-addr localhost:4221 -cluster-peers 'nats://localhost:4222/'
```

**Terminal 4 - Node 2**:
```bash
harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/'
```

Verify connectivity in the logs.

## Step 6: Test Replication

Verify data syncs between nodes:

1. **Add a Note in Node 1**:
   - Open http://localhost:8090/_/, go to "notes".
   - Click **New record**, enter:
     - Title: "Test Note"
     - Content: "This is a test"
     - Is Important: Check or leave unchecked
   - Save.

2. **Check Node 2**:
   - After a few seconds, visit http://localhost:8091/_/ > "notes".
   - Confirm "Test Note" appears.

3. **Test Bidirectional Sync**:
   - In Node 2, create a new note (e.g., "Node 2 Note").
   - Verify it appears in Node 1.

## Step 7: Test Fault Tolerance

Simulate a failure:

1. Stop Node 2 (Ctrl+C in Terminal 2).
2. Add a new note in Node 1.
3. Verify Node 1 works.
4. Restart Node 2:
   ```bash
   harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/'
   ```
5. Check http://localhost:8091/_/ > "notes" to confirm it syncs the new note.

## Real-World Application Ideas

- Distributed Task Manager
- Content Management System
- Inventory System
- Field Service App
- Edge Computing Dashboard

## Conclusion

You’ve built a preconfigured, distributed note-taking app with HarmonyLite and PocketBase, showcasing high availability and eventual consistency.

## Next Steps

- Explore [advanced configuration](configuration-reference.md)
- Learn [production deployment](production-deployment.md)
- Dive into [architecture](architecture.md)

