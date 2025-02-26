# Demo

This guide walks you through deploying **Pocketbase** as the backend and **HarmonyLite** for distributed SQLite replication. By combining these tools, you’ll build a fault-tolerant, scalable system where data sync across multiple nodes—perfect for small to medium-scale applications requiring high availability.

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

### Step 1: Configure Pocketbase

Each Pocketbase instance will connect to a HarmonyLite-managed database and run on a unique port to avoid conflicts.

1. **Start Pocketbase Instances with Unique Ports**  
   Run Pocketbase for each node, specifying the data directory and HTTP port:
   ```bash
   # Node 1: Listen on port 8090
   ./pocketbase serve --dir=/tmp/pb-1 --http=localhost:8090

   # Node 2: Listen on port 8091
   ./pocketbase serve --dir=/tmp/pb-2 --http=localhost:8091

   # Node 3: Listen on port 8092
   ./pocketbase serve --dir=/tmp/pb-3 --http=localhost:8092
   ```
   - **`--dir`**: Points to the data directory (e.g., `/tmp/pb-1`).
   - **`--http`**: Defines the address and port (e.g., `localhost:8090`).

2. **Admin Dashboards**  
   - Node 1: `http://localhost:8090/_/`
   - Node 2: `http://localhost:8091/_/`
   - Node 3: `http://localhost:8092/_/`
---

### Step 2: Set Up HarmonyLite Cluster

HarmonyLite will handle SQLite replication across three nodes. Each node requires a unique configuration and database file.

1. **Create Configuration Files**  
   For each node, create a `.toml` configuration file (e.g., `node-1-config.toml`, `node-2-config.toml`, `node-3-config.toml`). Here’s an example:
   ```toml
   # node-1-config.toml
   database_path = "/tmp/pb-1/data.db"
   ```
   Update `database_path` for nodes 2 and 3 (e.g., `/tmp/pb-2/data.db`, `/tmp/pb-3/data.db`).

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

### Step 3: Test Deployment and Replication

1. **Add a Note**  
   In one dashboard (e.g., `http://localhost:8090/_/`), register a super user.

2. **Verify Replication**  
   Visit the other nodes’ dashboards (e.g., `http://localhost:8091/_/` and `http://localhost:8092/_/`) to login.

---

## Conclusion

You’ve successfully deployed a distributed Pocketbase and HarmonyLite! This setup provides a solid foundation for real-world applications, combining a lightweight backend with robust replication. Feel free to enhance it with features like real-time updates or user authentication.
