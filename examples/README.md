# HarmonyLite Demo Examples

This directory contains scripts for demonstrating HarmonyLite's database replication capabilities using different applications and scenarios.

## Available Demos

### 1. Basic Cluster Demo

The `run-cluster.sh` script demonstrates a basic three-node HarmonyLite cluster with a simple Books database.

```bash
./run-cluster.sh
```

This script:
- Sets up a three-node HarmonyLite cluster
- Creates a sample Books database on each node
- Demonstrates basic replication functionality

### 2. PocketBase Demo

The `run-pocketbase-demo.sh` script automates the setup and execution of a distributed note-taking application using HarmonyLite for SQLite replication and PocketBase as the backend.

```bash
./run-pocketbase-demo.sh
```

This demo:
- Sets up a two-node HarmonyLite cluster
- Configures two PocketBase instances with:
  - Pre-created admin user
  - "notes" collection schema
- Demonstrates bidirectional replication between nodes
- Shows fault tolerance capabilities

#### PocketBase Demo Options

- `--help`: Display usage information
- `--keep-files`: Don't delete temporary files on exit
- `--pb-path PATH`: Specify an existing PocketBase binary
- `--no-download`: Don't download PocketBase if not found

#### Accessing the PocketBase Demo

When the demo is running, you can access:

- PocketBase Node 1: [http://localhost:8090/_/](http://localhost:8090/_/)
- PocketBase Node 2: [http://localhost:8091/_/](http://localhost:8091/_/)

Login with:
- Email: `admin@example.com`
- Password: `1234567890`

## Customizing Demos

You can modify the configuration files (e.g., `node-X-config.toml`) to explore different HarmonyLite settings or database schemas.

## Troubleshooting

If you encounter issues:

- Ensure required ports are available
- Check that the HarmonyLite binary is in your PATH or in the current directory
- Examine error messages in the terminal output
- See the [troubleshooting guide](../docs/docs/troubleshooting.md) for more help
