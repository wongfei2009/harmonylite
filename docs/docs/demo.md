# HarmonyLite Demo with PocketBase

This guide demonstrates setting up a distributed note-taking application using HarmonyLite for SQLite replication and **PocketBase** as the backend, with preconfigured admin and schema.

## Overview

In this demo, you'll:
- Set up a two-node HarmonyLite cluster for database replication
- Launch PocketBase instances preconfigured with an admin user and "notes" collection
- Test replication and fault tolerance in a distributed note-taking app

By the end, you'll see how HarmonyLite and PocketBase work together seamlessly.

## Prerequisites

- **HarmonyLite**: Downloaded and available in your PATH
- **PocketBase**: Downloaded from [PocketBase.io](https://pocketbase.io/docs/)
- A terminal and basic command-line skills
- About 10-15 minutes

## Step 1: Set Up Directory Structure

Create the demo directory and subdirectories:

```bash
mkdir -p harmonylite-demo/{pb-1,pb-2,user_pb_migrations}
cd harmonylite-demo
```

## Step 2: Preconfigure PocketBase

We'll initialize PocketBase with a default admin user and "notes" collection using a migration script.

1. **Create Admin User for Node 1**:
   ```bash
   ./pocketbase superuser create admin@example.com 1234567890 --dir=./pb-1
   ```

2. **Define "notes" Collection**:
   Create a migration file in `user_pb_migrations` (name it `init_notes.js`):
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
      // Down migration code (for reverting the changes)
      const collection = app.findCollectionByNameOrId("notes");
      return app.delete(collection);
   });
   ```

3. **Apply Migrations**:
   ```bash
   ./pocketbase migrate --dir=./pb-1 --migrationsDir ./user_pb_migrations
   ```

4. **Clone Configuration to Node 2**:
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
- Node 1: http://localhost:8090/_/ 
- Node 2: http://localhost:8091/_/ 
- Login credentials for both: `admin@example.com` / `1234567890`

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

You should see connectivity messages in the logs indicating successful cluster formation.

## Step 6: Test Replication

Verify data syncs between nodes:

1. **Add a Note in Node 1**:
   - Open http://localhost:8090/_/ and navigate to the "notes" collection
   - Click **New record** and enter:
     - Title: "Test Note"
     - Content: "This is a test"
     - Is Important: Toggle as desired
   - Save the record

2. **Check Node 2**:
   - After a few seconds, visit http://localhost:8091/_/ and open the "notes" collection
   - Confirm "Test Note" appears, demonstrating successful replication

3. **Test Bidirectional Sync**:
   - In Node 2, create a new note (e.g., "Node 2 Note")
   - Verify it appears in Node 1 after a brief sync period

## Step 7: Test Fault Tolerance

Simulate a node failure:

1. Stop Node 2 (Press Ctrl+C in Terminal 2)
2. Add a new note in Node 1 (e.g., "Offline Test")
3. Verify Node 1 continues to work normally
4. Restart Node 2:
   ```bash
   ./pocketbase serve --dir=./pb-2 --http=localhost:8091
   ```
   And in another terminal:
   ```bash
   harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/'
   ```
5. Check http://localhost:8091/_/ > "notes" to confirm it catches up and syncs the new note

## Real-World Applications

This architecture can be extended to build:

- Distributed Task Management Systems
- Content Management Systems with High Availability
- Inventory Systems with Offline Support
- Field Service Applications
- Edge Computing Dashboards

## Conclusion

You've successfully built a preconfigured, distributed note-taking application with HarmonyLite and PocketBase, showcasing high availability and eventual consistency in a simple yet powerful setup.

## Next Steps

- Explore [advanced configuration options](configuration-reference.md)
- Learn about [production deployment best practices](production-deployment.md)
- Dive into the [system architecture](architecture.md)