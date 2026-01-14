# Architecture

This document explains the core architecture, components, and design principles behind HarmonyLite. Understanding these concepts will help you better implement, configure, and troubleshoot your HarmonyLite deployment.

:::tip TL;DR
HarmonyLite is an AP system (Availability + Partition Tolerance) that uses SQLite triggers to capture changes, NATS JetStream to distribute them, and a last-writer-wins strategy for conflict resolution. Any node can accept writes, and all nodes eventually converge to the same state.
:::

:::note Reading Guide
This is the recommended starting point for understanding HarmonyLite. After this, read [Replication](replication.md) for details on how changes propagate, then [Snapshots](snapshots.md) for recovery mechanisms.
:::

## Architectural Overview

HarmonyLite implements a leaderless, eventually consistent replication system for SQLite databases. The architecture consists of four main components working together:

1. **Change Data Capture (CDC)**: Monitors and records database changes
2. **Message Distribution**: Publishes and subscribes to change events
3. **Change Application**: Applies changes to local databases
4. **State Management**: Handles snapshots and recovery

The following diagram illustrates the high-level architecture:

```mermaid
flowchart TB
    subgraph "Node 1"
        direction TB
        DB1[("SQLite DB")] <--> Triggers1["Triggers"]
        Triggers1 <--> ChangeLog1["Change Log Tables"]
        ChangeLog1 <--> HarmonyLite1["HarmonyLite Core"]
        HarmonyLite1 --- Health1["Health Check API"]
    end

    subgraph "NATS JetStream (External or Embedded)"
        Streams[("JetStream Streams")]
        Objects[("Object Store")]
    end

    subgraph "Node 2"
        direction TB
        HarmonyLite2["HarmonyLite Core"] <--> ChangeLog2["Change Log Tables"]
        ChangeLog2 <--> Triggers2["Triggers"]
        Triggers2 <--> DB2[("SQLite DB")]
        HarmonyLite2 --- Health2["Health Check API"]
    end

    HarmonyLite1 <--> Streams
    HarmonyLite2 <--> Streams
    HarmonyLite1 <--> Objects
    HarmonyLite2 <--> Objects

    classDef sqlite fill:#f9f,stroke:#333,stroke-width:2px
    classDef harmony fill:#bbf,stroke:#333,stroke-width:2px
    classDef nats fill:#bfb,stroke:#333,stroke-width:2px
    classDef component fill:#eee,stroke:#333,stroke-width:1px
    
    class DB1,DB2 sqlite
    class HarmonyLite1,HarmonyLite2,Health1,Health2 harmony
    class Streams,Objects nats
    class Triggers1,ChangeLog1,Triggers2,ChangeLog2 component
```

## Core Components

### 1. Change Data Capture (CDC)

HarmonyLite uses SQLite triggers to capture all database changes:

- **Triggers**: Automatically installed on all tables to detect INSERT, UPDATE, and DELETE operations
- **Change Log Tables**: Each monitored table has a corresponding `__harmonylite__<table_name>_change_log` table
- **Global Change Log**: A master table (`__harmonylite___change_log_global`) tracks the sequence of operations

When a change occurs:
1. The trigger fires and captures the change details
2. Information is stored in the change log table
3. A reference is added to the global change log

#### Table Structure

Database changes are tracked in specialized tables with this structure:

```mermaid
erDiagram
    USERS {
        int id PK
        string name
        string email
    }
    
    __harmonylite__change_log_global {
        int id PK
        int change_table_id FK
        string table_name
    }
    
    __harmonylite__users_change_log {
        int id PK
        int val_id
        string val_name
        string val_email
        string type
        int created_at
        int state
    }
    
    USERS ||--o{ __harmonylite__users_change_log : "triggers create"
    __harmonylite__users_change_log ||--o{ __harmonylite__change_log_global : "referenced by"
```

### 2. Message Distribution

HarmonyLite uses NATS JetStream for reliable message distribution:

- **Change Detection**: Monitors the database for modifications
- **Change Collection**: Retrieves pending records from change log tables
- **Hash Calculation**: Computes a hash from table name and primary keys
- **Stream Selection**: Routes changes to specific streams based on the hash
- **Publishing**: Sends changes to NATS JetStream
- **Confirmation**: Marks changes as published after acknowledgment

This approach ensures changes to the same row are always handled in order, while allowing parallel processing of changes to different rows.

### 3. Change Application

When a node receives a change message:

1. It checks if the change was originated locally (to avoid cycles)
2. It verifies the change hasn't been applied before
3. It parses the change details (table, operation type, values)
4. It applies the change to the local database
5. It records the message sequence for recovery tracking

### 4. State Management

HarmonyLite maintains system state through:

- **Sequence Map**: Tracks the last processed message for each stream
- **Snapshots**: Periodic database snapshots for efficient recovery
- **CBOR Serialization**: Efficient binary encoding for change records

### 5. Component Design

The internal package structure follows this design:

```mermaid
classDiagram
    direction TB
    class Main {
        +main()
        +changeListener()
    }
    class DB {
        +SqliteStreamDB
        +InstallCDC()
        +Replicate()
    }
    class LogStream {
        +Replicator
        +Publish()
        +Listen()
    }
    class Snapshot {
        +NatsDBSnapshot
        +SnapshotLeader
        +SaveSnapshot()
        +RestoreSnapshot()
    }
    class Health {
        +HealthChecker
        +HealthServer
    }
    
    Main --> DB : Uses
    Main --> LogStream : Uses
    Main --> Snapshot : Uses
    Main --> Health : Uses
    
    LogStream --> Snapshot : Manages
    LogStream ..> DB : Reads/Writes
    Snapshot ..> DB : Backups
    Health ..> DB : Checks
    Health ..> LogStream : Checks
```

## Sequence Map & Idempotency

The **Sequence Map** is the "brain" of HarmonyLite's reliability. It is a local file (default: `seq-map.cbor`) that maintains the state of consumption for every stream.

### Why is it critical?

1.  **Idempotency (Exactly-Once Processing)**: 
    *   In distributed systems, messages may be delivered more than once.
    *   The Sequence Map filters out duplicates by ensuring we only process `Sequence > StartSequence`.
    *   This guarantees that a database change (like an INSERT) is never applied twice, which would corrupt data.

2.  **Crash Recovery**:
    *   If a node restarts, it reads the Sequence Map to know *exactly* where it left off.
    *   It resumes consumption from `LastSequence + 1`.

### How it works

The Sequence Map is a simple Key-Value store serialized in **CBOR** (Concise Binary Object Representation) for performance and compactness.

*   **Key**: Stream Name (e.g., `harmonylite-changes-1`)
*   **Value**: Last successfully applied Sequence Number (e.g., `1042`)

```mermaid
flowchart LR
    Msg["Incoming Message<br/>Seq: 105"] --> Check{Check Map}
    
    Check -->|Last Seq: 104| Process[Process Message]
    Check -->|Last Seq: 105| Ignore["Drop (Duplicate)"]
    
    Process --> Apply[Apply to DB]
    Apply --> Update["Update Map<br/>Set Seq: 105"]
    Update --> Persist["Flush to Disk<br/>seq-map.cbor"]
```

## Key Mechanisms

### Leaderless Replication

Unlike leader-follower systems, HarmonyLite operates without a designated leader:

- Any node can accept writes
- Changes propagate to all nodes
- No single point of failure
- Higher write availability

### Eventual Consistency

HarmonyLite prioritizes availability over immediate consistency:

- Changes eventually reach all nodes
- Last-writer-wins conflict resolution
- No global locking mechanism
- Non-blocking operations

### Sharding

Change streams can be sharded to improve performance:

- Each shard handles a subset of rows
- Determined by hashing table name and primary keys
- Enables parallel processing
- Configurable via `replication_log.shards`

```mermaid
flowchart LR
    Change[Change Event] --> Hash{"Hash(Table + PK)"}
    Hash -->|Hash % N = 0| Stream0[Stream-0]
    Hash -->|Hash % N = 1| Stream1[Stream-1]
    Hash -->|...| StreamN[Stream-N]
    
    Stream0 --> Consumer0[Consumer-0]
    Stream1 --> Consumer1[Consumer-1]
    StreamN --> ConsumerN[Consumer-N]
    
    Consumer0 --> Serial[Strict Serial Processing]
    Consumer1 --> Serial
    ConsumerN --> Serial
```

### Message Flow

The complete message flow looks like this:

```mermaid
sequenceDiagram
    participant App1 as Application
    participant DB1 as SQLite DB
    participant HL1 as HarmonyLite
    participant NATS as NATS JetStream
    participant HL2 as HarmonyLite Remote
    participant DB2 as SQLite DB Remote
    
    Note over App1, DB1: Local Write Operation
    App1->>DB1: INSERT/UPDATE/DELETE
    DB1->>DB1: Trigger captures change
    DB1->>DB1: Write to change_log
    
    Note over HL1, NATS: Replication Process
    HL1->>DB1: Poll/Watch changes
    DB1->>HL1: New Change Event
    HL1->>HL1: Compute Shard Hash
    HL1->>HL1: Compress Payload (ZSTD)
    HL1->>NATS: Publish to Subject (Shard)
    NATS-->>HL1: ACK
    HL1->>DB1: Mark as Published
    
    Note over NATS, DB2: Remote Application
    NATS->>HL2: Push Message
    HL2->>HL2: Decompress
    HL2->>DB2: Apply Change (Tx)
    HL2->>DB2: Update Sequence Map
    HL2-->>NATS: ACK
```

## Snapshot and Recovery

### Snapshot Creation

Snapshots provide efficient node recovery:

```mermaid
graph TB
    Start([Start]) --> LeaderEx{Is Leader?}
    LeaderEx -->|No| Wait[Wait / Follower Mode]
    LeaderEx -->|Yes| CheckTime{Interval Passed?}
    
    CheckTime -->|No| Wait
    CheckTime -->|Yes| Init[Init Snapshot]
    
    Init --> Backup["VACUUM INTO (Temp File)"]
    Backup --> Sanitize[Remove Triggers & Logs]
    Sanitize --> Compress[Compress/Optimize]
    
    Compress --> Upload{Upload To Storage}
    Upload -->|NATS| ObjStore[Object Store]
    Upload -->|S3| S3Bucket[S3 Bucket]
    Upload -->|SFTP/WebDAV| FileStore[File Storage]
    
    ObjStore --> Finish([Update Sequence Map])
    S3Bucket --> Finish
    FileStore --> Finish
```

### Node Recovery

When a node starts or needs to catch up:

```mermaid
graph TB
    I[Node Startup] --> J[Check DB Integrity]
    J --> K[Load Sequence Map]
    K --> L{Too Far Behind?}
    L -->|No| R[Normal Operation]
    L -->|Yes| M[Download Snapshot]
    
    M --> Source{Select Source}
    Source -->|NATS| Obj[Object Store]
    Source -->|S3| S3[S3 Bucket]
    Source -->|SFTP/WebDAV| File[File Storage]
    
    Obj --> N
    S3 --> N
    File --> N
    
    N[Exclusive Lock] --> O[Replace DB Files]
    O --> P[Install Triggers]
    P --> Q[Process Recent Changes]
    Q --> R
```

## Understanding Trade-offs

### CAP Theorem Positioning

HarmonyLite makes specific trade-offs according to the CAP theorem:

- **Consistency**: Eventual (not strong)
- **Availability**: High (prioritized)
- **Partition Tolerance**: Maintained

This positions HarmonyLite as an AP system (Availability and Partition Tolerance) rather than a CP system.

### Suitable Use Cases

HarmonyLite is ideal for:
- Read-heavy workloads
- Systems that can tolerate eventual consistency
- Applications needing high write availability
- Edge computing and distributed systems

### Less Suitable Use Cases

HarmonyLite may not be the best choice for:
- Strong consistency requirements
- Complex transactional workloads
- Financial systems requiring immediate consistency
- Systems with strict ordering requirements

## Performance Characteristics

### Scalability

- **Read Scalability**: Excellent (horizontal)
- **Write Scalability**: Good (limited by conflict resolution)
- **Node Count**: Practical up to dozens of nodes

### Latency

- **Local Operations**: Minimal impact (~1-5ms overhead)
- **Replication Delay**: Typically 50-500ms depending on network
- **Recovery Time**: Proportional to changes since last snapshot

### Resource Usage

- **Memory**: Moderate (configurable)
- **CPU**: Low to moderate
- **Disk**: Additional space for change logs and snapshots
- **Network**: Proportional to change volume and compression settings

## Next Steps

- [Replication Details](replication.md) - Deep dive into the replication process
- [Snapshots](snapshots.md) - How snapshots and recovery work
- [Configuration Reference](configuration-reference.md) - Complete configuration options