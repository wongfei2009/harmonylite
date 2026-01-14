# Frequently Asked Questions

Welcome to the HarmonyLite FAQ! This guide covers everything from basic concepts to advanced deployment scenarios.

## Top 5 "Need to Know"

1.  **Is this drop-in compatible with SQLite?**
    > Yes! You just need to enable `PRAGMA trusted_schema = ON;`. No other code changes required.

2.  **How are conflicts handled?**
    > **Last-Writer-Wins**. If two nodes update the same row, the update with the later timestamp wins.

3.  **Do I need an external NATS server?**
    > No. HarmonyLite has an embedded NATS server for simple setups (3-5 nodes). For larger clusters, an external NATS is recommended.

4.  **Is it strong or eventual consistency?**
    > **Eventual Consistency**. Writes are local and fast, then replicated asynchronously.

5.  **Can I read from any node?**
    > **Yes!** All nodes are full read/write replicas (unless configured otherwise).

---

## Table of Contents

- [General Questions](#general-questions)
- [Technical Deep Dive](#technical-deep-dive)
- [Deployment & Operations](#deployment--operations)
- [Performance & Scaling](#performance--scaling)
- [Troubleshooting](#troubleshooting)

---

## General Questions

### What is HarmonyLite?
HarmonyLite is a **distributed SQLite replication system**. It allows you to run SQLite on multiple nodes, where each node can accept writes, and changes are magically synchronized to all other nodes. It's essentially "SQLite with superpowers."

### How is this different from rqlite, dqlite, or LiteFS?
Great question! The main difference is our **Leaderless** architecture vs. their **Leader-Follower** approach.

| Feature | HarmonyLite | rqlite | dqlite | LiteFS |
| :--- | :--- | :--- | :--- | :--- |
| **Architecture** | **Leaderless** (Multi-Master) | Leader-Follower | Leader-Follower | Primary-Replica |
| **Writes** | **Any Node** | Leader Only | Leader Only | Primary Only |
| **Consistency** | Eventual | Strong (Raft) | Strong (Raft) | Strong (Linearizable) |
| **Setup** | Sidecar / Binary | Replaced SQLite | Replaced SQLite | VFS / Fuse |

**Choose HarmonyLite if:** You want high availability for writes and can tolerate eventual consistency.

### When should I use HarmonyLite?
- **High Write Availability**: You need to write even if the "leader" is down (because there is no leader!).
- **Edge Computing**: Nodes are geographically distributed and need to work offline occasionally.
- **Read Scaling**: You want to distribute read queries across many nodes.
- **Zero-Code Change**: You don't want to rewrite your SQL queries or application logic.

### What are the trade-offs?
- **Consistency**: It is *eventual*. You might read stale data for a few milliseconds after a write on another node.
- **Conflicts**: If two people edit the same row at the exact same time, one change will overwrite the other (last-writer-wins).
- **Transactions**: Distributed transactions are not supported. Transactions are local to the node.

---

## Technical Deep Dive

### How does replication actually work?
It uses a clever combination of SQLite Triggers and NATS JetStream:
1.  **Capture**: Triggers in your database capture every `INSERT`, `UPDATE`, and `DELETE`.
2.  **Log**: These changes are saved to internal tracking tables (`_hl_log`).
3.  **Publish**: HarmonyLite reads the log and pushes messages to NATS.
4.  **Apply**: Other nodes subscribe, receive the message, and execute the SQL locally.

For more details, see [Architecture & Replication Concepts](architecture.md).

### What happens to conflicts?
We strictly follow a **Last-Writer-Wins (LWW)** policy based on hybrid logical clocks.
- Row `A` modified by Node 1 at `10:00:01`
- Row `A` modified by Node 2 at `10:00:02`
- **Result**: Node 2's change overwrites Node 1's change on *both* nodes.

### What version of SQLite do I need?
**SQLite 3.35.0+** is required. We use the `RETURNING` clause heavily for our triggers.

---

## Deployment & Operations

### Can I run this in Kubernetes / Docker?
**Absolutely.**
- **Docker**: Run it as a sidecar container sharing a volume with your app.
- **Kubernetes**: Use a `StatefulSet` to give each node a stable identity.

See [Production Deployment Guide](production-deployment.md) for examples.

### Do I need to buy/manage a NATS server?
- **Small Clusters (3-5 nodes)**: Just use the **embedded** NATS server. It's built-in!
- **Large Clusters / Production**: We recommend an external NATS cluster for better monitoring and resilience.
See [NATS Configuration](nats-configuration.md).

### How do I handle backups?
1.  **Snapshots**: Configure the `[snapshot]` section to auto-backup to S3 or disk.
2.  **Standard Backup**: Just copy the `.sqlite` file! (Ideally use the SQLite `.backup` API).
3.  **Passive Node**: Run an extra node solely for backup purposes.

See [Snapshots & Recovery](snapshots.md).

### Can I have Read-Only Replicas?
Yes. Set `publish = false` in the configuration for that node. It will receive all updates but never send any of its own (effectively becoming a read-replica).

---

## Performance & Scaling

### How many nodes can I have?
- **Tested**: 3-10 nodes is the sweet spot.
- **Theoretical**: NATS scales well, so dozens of nodes is possible, but write conflicts become more likely as node count increases.

### What is the performance overhead?
- **Writes**: Slightly slower (~10-20%) because triggers need to write to the `_hl_log` table.
- **Reads**: **Zero overhead**. Reads go directly to the raw SQLite file.

### Does it support Sharding?
**No.** Every node has a full copy of the dataset.

---

## Troubleshooting

### My changes aren't replicating!
Check these first:
1.  **Pragma**: Did you run `PRAGMA trusted_schema = ON;`?
2.  **Triggers**: Run `harmonylite -cleanup` to reinstall triggers.
3.  **NATS**: Can the nodes reach each other? (Check ports 4222/6222).

### I see "database is locked" errors
This is usually an application-side issue.
- Ensure you use **WAL Mode**: `PRAGMA journal_mode = WAL;`.
- Set a **Busy Timeout**: `PRAGMA busy_timeout = 5000;` (5s).

For deeper debugging, see [Troubleshooting Guide](troubleshooting.md).

### How to recover from corruption?
If `PRAGMA integrity_check` fails:
1.  Stop HarmonyLite.
2.  Delete the corrupted `.sqlite` file on the bad node.
3.  Restart HarmonyLite.
4.  It will automatically request a [Snapshot](snapshots.md) from a healthy peer and rebuild itself.

---

*Still have questions? Check the [Discussions](https://github.com/wongfei2009/harmonylite/discussions) or open an [Issue](https://github.com/wongfei2009/harmonylite/issues).*