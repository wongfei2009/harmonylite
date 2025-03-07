# HarmonyLite Architecture

This document explains the core architecture, components, and design principles behind HarmonyLite. Understanding these concepts will help you better implement, configure, and troubleshoot your HarmonyLite deployment.

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
        DB1[SQLite DB] --> Triggers1[SQLite Triggers]
        Triggers1 --> ChangeLog1[Change Log]
        ChangeLog1 --> HarmonyLite1[HarmonyLite]
    end

    subgraph "NATS JetStream"
        Streams[(JetStream Streams)]
        Objects[(Object Store)]
    end

    subgraph "Node 2"
        HarmonyLite2[HarmonyLite] --> ChangeLog2[Change Log]
        ChangeLog2 --> Triggers2[SQLite Triggers]
        Triggers2 --> DB2[SQLite DB]
    end

    HarmonyLite1 <--> Streams
    HarmonyLite2 <--> Streams
    HarmonyLite1 <--> Objects
    HarmonyLite2 <--> Objects

    classDef sqlite fill:#f9f,stroke:#333,stroke-width:2px
    classDef harmony fill:#bbf,stroke:#333,stroke-width:2px
    classDef nats fill:#bfb,stroke:#333,stroke-width:2px
    
    class DB1,DB2 sqlite
    class HarmonyLite1,HarmonyLite2 harmony
    class Streams,Objects nats
```

## Core Components

### 1. Change Data Capture (CDC)

HarmonyLite uses SQLite triggers to capture all database changes:

- **Triggers**: Automatically installed on all tables to detect INSERT, UPDATE, and DELETE operations
- **Change Log Tables**: Each monitored table has a corresponding `__harmonylite__<table_name>_change_log` table
- **Global Change Log**: A master table (`__harmonylite___global_change_log`) tracks the sequence of operations

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
    
    GLOBAL_CHANGE_LOG {
        int id PK
        int change_table_id FK
        string table_name
    }
    
    USERS_CHANGE_LOG {
        int id PK
        int val_id
        string val_name
        string val_email
        string type
        int created_at
        int state
    }
    
    USERS ||--o{ USERS_CHANGE_LOG : "triggers create"
    USERS_CHANGE_LOG ||--o{ GLOBAL_CHANGE_LOG : "references"
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

### Message Flow

The complete message flow looks like this:

```mermaid
sequenceDiagram
    participant App1 as Application (Node 1)
    participant DB1 as SQLite DB (Node 1)
    participant HL1 as HarmonyLite (Node 1)
    participant NATS as NATS JetStream
    participant HL2 as HarmonyLite (Node 2)
    participant DB2 as SQLite DB (Node 2)
    participant App2 as Application (Node 2)
    
    App1->>DB1: INSERT/UPDATE/DELETE
    DB1->>DB1: Trigger executes
    DB1->>DB1: Record in change log
    
    HL1->>DB1: Poll for changes
    DB1->>HL1: Return pending changes
    HL1->>HL1: Calculate hash
    HL1->>NATS: Publish to stream
    NATS->>HL1: Acknowledge receipt
    HL1->>DB1: Mark as published
    
    NATS->>HL2: Deliver change
    HL2->>HL2: Process change
    HL2->>DB2: Apply change
    HL2->>NATS: Acknowledge processing
    
    App2->>DB2: Read updated data
    DB2->>App2: Return data
```

## Snapshot and Recovery

### Snapshot Creation

Snapshots provide efficient node recovery:

```mermaid
graph TB
    A[Monitor Sequence Numbers] --> B{Threshold Reached?}
    B -->|Yes| C[Create Temp Directory]
    B -->|No| A
    C --> D["VACUUM INTO (Temp Copy)"]
    D --> E[Remove Triggers & Logs]
    E --> F[Optimize with VACUUM]
    F --> G[Upload Snapshot]
    G --> H[Update Sequence Map]
    H --> A
```

### Node Recovery

When a node starts or needs to catch up:

```mermaid
graph TB
    I[Node Startup] --> J[Check DB Integrity]
    J --> K[Load Sequence Map]
    K --> L{Too Far Behind?}
    L -->|Yes| M[Download Snapshot]
    L -->|No| R[Normal Operation]
    M --> N[Exclusive Lock]
    N --> O[Replace DB Files]
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