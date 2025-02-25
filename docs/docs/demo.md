---
id: demo
title: Demo
slug: /demo
---

# Demo

This page demonstrates HarmonyLite in action with a practical example of distributed SQLite replication.

## Demo Overview

In this demo, we'll show how HarmonyLite enables:

1. Multi-node SQLite replication with no primary server
2. Automatic synchronization between database instances
3. Handling of concurrent writes from different locations

## Setting Up the Demo Environment

Let's create a simple three-node HarmonyLite cluster locally to see how changes propagate:

```bash
# Create the demo databases with sample book data
./harmonylite -config examples/node-1-config.toml -cluster-addr localhost:4221 &
./harmonylite -config examples/node-2-config.toml -cluster-addr localhost:4222 -cluster-peers 'nats://localhost:4221/' &
./harmonylite -config examples/node-3-config.toml -cluster-addr localhost:4223 -cluster-peers 'nats://localhost:4221/,nats://localhost:4222/' &
```

After starting the cluster, we'll have three SQLite databases:
- `/tmp/harmonylite-1.db`
- `/tmp/harmonylite-2.db`
- `/tmp/harmonylite-3.db`

## Testing Replication

Now let's see replication in action by making changes to one node and observing them propagate to others:

```bash
# Insert a new book on node 1
sqlite3 /tmp/harmonylite-1.db
> INSERT INTO Books (title, author, publication_year) VALUES ('Dune', 'Frank Herbert', 1965);
```

Within seconds, you can verify the change has propagated to the other nodes:

```bash
# Check node 2
sqlite3 /tmp/harmonylite-2.db "SELECT * FROM Books WHERE title='Dune';"

# Check node 3
sqlite3 /tmp/harmonylite-3.db "SELECT * FROM Books WHERE title='Dune';"
```

## Concurrent Write Handling

HarmonyLite uses a "last-writer-wins" strategy to handle concurrent writes. Let's demonstrate this:

```bash
# First, get a book ID to update
BOOK_ID=$(sqlite3 /tmp/harmonylite-1.db "SELECT id FROM Books LIMIT 1;")

# Update the same book on different nodes nearly simultaneously
sqlite3 /tmp/harmonylite-1.db "UPDATE Books SET title='Updated on Node 1' WHERE id=$BOOK_ID;"

# Quickly run this on another terminal
sqlite3 /tmp/harmonylite-2.db "UPDATE Books SET title='Updated on Node 2' WHERE id=$BOOK_ID;"
```

After a moment, check all three nodes:

```bash
sqlite3 /tmp/harmonylite-1.db "SELECT title FROM Books WHERE id=$BOOK_ID;"
sqlite3 /tmp/harmonylite-2.db "SELECT title FROM Books WHERE id=$BOOK_ID;"
sqlite3 /tmp/harmonylite-3.db "SELECT title FROM Books WHERE id=$BOOK_ID;"
```

All three should display the same title, which will be the "last-writer" based on JetStream's RAFT consensus.

## Snapshots and Recovery

HarmonyLite can also take snapshots to support faster node recovery. Let's demonstrate:

```bash
# Stop node 3
kill %3

# Take a snapshot with node 1
./harmonylite -config examples/node-1-config.toml -save-snapshot

# Make a change that node 3 will miss while offline
sqlite3 /tmp/harmonylite-1.db "INSERT INTO Books (title, author, publication_year) VALUES ('New book while offline', 'Some Author', 2023);"

# Restart node 3 - it will automatically recover from snapshot and then catch up with missed changes
./harmonylite -config examples/node-3-config.toml -cluster-addr localhost:4223 -cluster-peers 'nats://localhost:4221/,nats://localhost:4222/' &

# Verify node 3 has the latest data
sqlite3 /tmp/harmonylite-3.db "SELECT title FROM Books WHERE title='New book while offline';"
```