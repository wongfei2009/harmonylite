# HarmonyLite

[![Go Report Card](https://goreportcard.com/badge/github.com/wongfei2009/harmonylite)](https://goreportcard.com/report/github.com/wongfei2009/harmonylite)
![GitHub](https://img.shields.io/github/license/wongfei2009/harmonylite)
[![Tests](https://github.com/wongfei2009/harmonylite/actions/workflows/tests.yml/badge.svg)](https://github.com/wongfei2009/harmonylite/actions/workflows/tests.yml)

## What is HarmonyLite?

HarmonyLite is a distributed SQLite replication system with leaderless architecture and eventual consistency. It enables robust multi-directional replication between nodes using [NATS JetStream](https://nats.io/).

*This project is a fork of [Marmot](https://github.com/maxpert/marmot), which appears to be no longer maintained.*

## Why HarmonyLite?

HarmonyLite continues and extends the vision of Marmot by providing:

- Leaderless, eventually consistent SQLite replication
- Easy horizontal scaling for read-heavy SQLite applications
- Minimal configuration with a modern, maintained codebase
- Enhanced testing and reliability
- Active development and community support

## Quick Start

Download the [latest release](https://github.com/YOUR_USERNAME/harmonylite/releases/latest) and extract:

```bash
tar vxzf harmonylite-v*.tar.gz
```

From the extracted directory, run the example cluster:

```bash
./examples/run-cluster.sh
```

Make changes to one database and watch them propagate:

```bash
# Insert data in the first database
sqlite3 /tmp/harmonylite-1.db
> PRAGMA trusted_schema = ON;
> INSERT INTO Books (title, author, publication_year) VALUES ('Project Hail Mary', 'Andy Weir', 2021);

# See it appear in the second database
sqlite3 /tmp/harmonylite-2.db
> SELECT * FROM Books;
```

## What Makes HarmonyLite Different?

Unlike other SQLite replication solutions that require a leader-follower architecture, HarmonyLite:

- Has **no primary node** - any node can write to its local database
- Operates with **eventual consistency** - no global locking or blocking
- Requires **no changes** to your existing SQLite application code
- Runs as a **sidecar** to your existing processes

## Features

![Eventually Consistent](https://img.shields.io/badge/Eventually%20Consistent-✔️-green)
![Leaderless Replication](https://img.shields.io/badge/Leaderless%20Replication-✔️-green)
![Fault Tolerant](https://img.shields.io/badge/Fault%20Tolerant-✔️-green)
![Built on NATS](https://img.shields.io/badge/Built%20on%20NATS-✔️-green)

- Multiple snapshot storage options:
  - NATS Blob Storage
  - WebDAV
  - SFTP
  - S3 Compatible (AWS S3, Minio, Blackblaze, SeaweedFS)
- Embedded NATS server
- Log compression for content-heavy applications
- Sleep timeout support for serverless environments
- Comprehensive E2E testing

## Project Roadmap

Future plans for HarmonyLite include:
- Improved documentation and examples

## Documentation

For detailed documentation, see the [docs directory](./docs).

## CLI Documentation

HarmonyLite is designed for simplicity with minimal configuration. Key command line options:

- `config` - Path to TOML configuration file
- `cleanup` - Clean up hooks and exit
- `save-snapshot` - Create and upload a snapshot
- `cluster-addr` - Binding address for cluster
- `cluster-peers` - Comma-separated list of NATS peers
- `leaf-server` - Leaf node connection list

See `config.toml` for detailed configuration options.

## Development

### Prerequisites

- Go 1.22 or later
- SQLite development libraries

### Building from Source

```bash
export CGO_CFLAGS="-Wno-typedef-redefinition -Wno-nullability-completeness"
go build
```

### Running Tests

```bash
ginkgo tests/e2e
```

## License

MIT License - See [LICENSE](LICENSE) for details.

## Acknowledgements

This project is a fork of [Marmot](https://github.com/maxpert/marmot) by Zohaib Sibte Hassan. We are grateful for the solid foundation provided by the original project.