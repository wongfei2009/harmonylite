# Demo

This guide walks you through deploying a simple note-taking application using **Pocketbase** as the backend and **HarmonyLite** for distributed SQLite replication. By combining these tools, you’ll build a fault-tolerant, scalable system where notes sync across multiple nodes—perfect for small to medium-scale applications requiring high availability.

Let’s get started!

---

## Introduction

**Pocketbase** is an open-source backend-in-one solution featuring an embedded SQLite database, real-time subscriptions, authentication, file storage, and a built-in admin dashboard—all packaged in a single executable. **HarmonyLite** complements it by enabling distributed replication of SQLite databases across multiple nodes using a NATS-based cluster and a "last-writer-wins" conflict resolution strategy.

In this demo, we’ll:
- Set up a three-node HarmonyLite cluster to replicate the database.
- Configure Pocketbase instances to use these replicated databases, each running on a unique port.
- Use Pocketbase’s admin dashboard as the app and test replication and fault tolerance.

The outcome is a practical, distributed note-taking app ready for real-world use!

---

## Prerequisites

Before starting, ensure you have:
- **HarmonyLite**: Installed and available (download or build from source per its documentation).
- **Pocketbase**: Downloaded from [Pocketbase.io](https://pocketbase.io/docs/) for your operating system.
- A terminal and basic command-line knowledge.

---

## Step-by-Step Deployment Guide

### Step 1: Set Up HarmonyLite Cluster

HarmonyLite will handle SQLite replication across three nodes. Each node requires a unique configuration and database file.

1. **Create Configuration Files**  
   For each node, create a `.toml` configuration file (e.g., `node-1-config.toml`, `node-2-config.toml`, `node-3-config.toml`). Here’s an example:
   ```toml
   # node-1-config.toml
   database_path = "/tmp/harmonylite-1.db"
   ```
   Update `database_path` for nodes 2 and 3 (e.g., `/tmp/harmonylite-2.db`, `/tmp/harmonylite-3.db`).

2. **Start the Nodes**  
   Launch each node in a separate terminal window:
   ```bash
   # Node 1
   ./harmonylite -config node-1-config.toml -cluster-addr localhost:4221 &

   # Node 2 (connects to Node 1)
   ./harmonylite -config node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/' &

   # Node 3 (connects to Nodes 1 and 2)
   ./harmonylite -config node-3-config.toml -cluster-addr localhost:4223 -cluster-peers 'nats://localhost:4221/,nats://localhost:4222/' &
   ```
   - `-cluster-addr`: Unique address for each node.
   - `-cluster-peers`: List of peer nodes for replication.

3. **Verify Cluster**  
   Confirm all nodes are running and communicating by checking their logs for errors.

---

### Step 2: Configure Pocketbase

Each Pocketbase instance will connect to a HarmonyLite-managed database and run on a unique port to avoid conflicts.

1. **Start Pocketbase Instances with Unique Ports**  
   Run Pocketbase for each node, specifying the database directory and HTTP port:
   ```bash
   # Node 1: Listen on port 8090
   ./pocketbase serve --dir=/tmp/harmonylite-1.db --http=localhost:8090

   # Node 2: Listen on port 8091
   ./pocketbase serve --dir=/tmp/harmonylite-2.db --http=localhost:8091

   # Node 3: Listen on port 8092
   ./pocketbase serve --dir=/tmp/harmonylite-3.db --http=localhost:8092
   ```
   - **`--dir`**: Points to the SQLite database file (e.g., `/tmp/harmonylite-1.db`).
   - **`--http`**: Defines the address and port (e.g., `localhost:8090`).

2. **Access Admin Dashboards**  
   Open each instance’s admin dashboard in a browser:
   - Node 1: `http://localhost:8090/_/`
   - Node 2: `http://localhost:8091/_/`
   - Node 3: `http://localhost:8092/_/`

3. **Create a Notes Collection**  
   Using any node’s dashboard (e.g., Node 1 at `http://localhost:8090/_/`):
   - Create a new collection called `notes`.
   - Add fields:
     - `title` (Text)
     - `content` (Text)
   - Save the collection.

---

### Step 3: Test Deployment and Replication

1. **Add a Note**  
   In one dashboard (e.g., `http://localhost:8090/_/`), go to the `notes` collection and create a note (e.g., "Test Note" with "Hello, world!").

2. **Verify Replication**  
   Visit the other nodes’ dashboards (e.g., `http://localhost:8091/_/` and `http://localhost:8092/_/`) to ensure the note appears on each.

3. **Simulate Failure**  
   - Stop one node (e.g., terminate Node 2’s process).
   - Add a new note on a running node (e.g., Node 1).
   - Restart the stopped node and confirm it syncs the new note.

This tests fault tolerance and data consistency across the cluster!

---

## Benefits of This Setup

- **High Availability**: If one node goes down, others keep the app operational.
- **Scalability**: Easily add more nodes to handle increased load.
- **Ease of Use**: Pocketbase’s dashboard simplifies management, while HarmonyLite automates replication.

---

## Conclusion

You’ve successfully deployed a distributed note-taking app with Pocketbase and HarmonyLite! This setup provides a solid foundation for real-world applications, combining a lightweight backend with robust replication. Feel free to enhance it with features like real-time updates or user authentication.
