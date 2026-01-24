# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

HarmonyLite is a distributed SQLite replication system with a leaderless architecture and eventual consistency. It enables multi-directional replication between nodes using NATS JetStream as the message broker. It runs as a sidecar process requiring no changes to existing SQLite applications.

## Build and Development Commands

```bash
# Build binary for current platform
make build

# Build statically linked binary
make build-static

# Run all tests (unit + integration)
make test

# Run E2E tests only
go run github.com/onsi/ginkgo/v2/ginkgo -v tests/e2e

# Run unit tests only (exclude E2E)
go test -v $(go list ./... | grep -v "/tests/e2e")

# Run a single test file
go test -v ./db/ -run TestSchemaCache

# Run a single E2E test
go run github.com/onsi/ginkgo/v2/ginkgo -v --focus "should pause replication" tests/e2e

# macOS CGO workaround (if you see nullability-completeness warnings)
CGO_CFLAGS="-O2 -g -Wno-nullability-completeness" go test ./...

# Clean build artifacts
make clean
```

## Architecture

The system has four main components:

### 1. Change Data Capture (`db/`)
- `SqliteStreamDB` wraps SQLite with CDC capabilities via triggers
- Changes stored in `__harmonylite__<table>_change_log` tables
- `SchemaCache` computes deterministic SHA-256 schema hashes
- CBOR serialization for efficient event encoding

### 2. Message Distribution (`stream/`)
- NATS JetStream for persistent change streams
- Supports embedded NATS server or external cluster
- Object Store used for snapshots
- Zstd compression for wire protocol

### 3. Replication Engine (`logstream/`)
- `Replicator` orchestrates consuming and applying changes
- Schema validation before applying events (pause on mismatch)
- Last-writer-wins conflict resolution
- `SchemaRegistry` tracks cluster-wide schema versions

### 4. Snapshot System (`snapshot/`)
- Pluggable backends: NATS Object Store, S3, WebDAV, SFTP
- Used for new node initialization and recovery
- Leader election for snapshot creation

### Key Dependency Flow
```
harmonylite.go (main)
  ├── cfg/        (TOML configuration)
  ├── db/         (SQLite + CDC)
  ├── logstream/  (Replicator)
  │   └── stream/ (NATS client)
  ├── snapshot/   (persistence backends)
  ├── pool/       (connection pooling)
  └── telemetry/  (Prometheus metrics)
```

## Key Design Patterns

- **Eventual Consistency**: No global locking; any node can write locally
- **Schema Versioning**: Nodes pause replication on schema mismatch, resume when schemas match
- **Previous Schema Hash**: Supports rolling upgrades by accepting both current and previous schema versions
- **Connection Pooling**: `pool.SqlitePool` manages SQLite connections with WAL mode
- **Trigger-based CDC**: SQL triggers capture changes inline with writes

## Testing

- Unit tests: `*_test.go` files throughout packages
- E2E tests: `tests/e2e/` using Ginkgo v2 framework
- E2E tests spin up multi-node clusters with embedded NATS

## Configuration

See `config.toml` for example configuration. Key sections:
- `db_path`: SQLite database location
- `node_id`: Unique node identifier
- `[nats]`: NATS connection settings
- `[snapshot]`: Snapshot backend configuration
- `[replication_log]`: JetStream stream settings
