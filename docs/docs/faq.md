# Frequently Asked Questions

## General Questions

### What is HarmonyLite?

HarmonyLite is a distributed SQLite replication system designed to provide leaderless, eventually consistent replication across multiple nodes. It enables SQLite databases to synchronize changes between multiple instances without requiring a central coordinator.

### How does HarmonyLite differ from other SQLite replication solutions?

Unlike other solutions like rqlite, dqlite, or LiteFS which use a leader-follower architecture, HarmonyLite allows writes on any node (leaderless) and provides eventual consistency. This approach increases write availability at the cost of strong consistency guarantees.

| Feature | HarmonyLite | rqlite | dqlite | LiteFS |
|---------|------------|--------|--------|--------|
| Architecture | Leaderless | Leader-follower | Leader-follower | Primary-replica |
| Consistency | Eventual | Strong | Strong | Strong |
| Write Nodes | All nodes | Leader only | Leader only | Primary only |
| Application Changes | None | API changes | API changes | VFS layer |
| Replication Level | Logical (row) | Logical (SQL) | Physical | Physical |

### When should I use HarmonyLite?

HarmonyLite is ideal for:
- Applications that need high write availability
- Read-heavy workloads that benefit from distributing reads
- Edge computing scenarios where nodes may operate independently
- Systems that can tolerate eventual consistency
- Applications using SQLite that need replication without code changes

### What are the limitations of HarmonyLite?

HarmonyLite has some trade-offs to consider:
- Eventual (not strong) consistency guarantees
- Potential for conflicts in write-heavy scenarios
- No built-in conflict resolution beyond last-writer-wins
- Not ideal for financial systems or applications requiring strict transaction ordering
- Some overhead from triggers and change tracking

## Technical Questions

### How does replication work in HarmonyLite?

HarmonyLite uses a multi-step process for replication:
1. SQLite triggers capture database changes
2. Changes are logged in special tracking tables
3. HarmonyLite publishes changes to NATS JetStream
4. Other nodes subscribe to these changes
5. Changes are applied on receiving nodes
6. A last-writer-wins strategy resolves conflicts

### What is the maximum number of nodes supported?

HarmonyLite has been tested with dozens of nodes. The practical limit depends on:
- Network bandwidth and latency
- Write volume and patterns
- Hardware resources
- NATS JetStream configuration

For most use cases, 3-20 nodes provides a good balance of availability and performance.

### Does HarmonyLite require code changes to my application?

No, HarmonyLite works as a sidecar process without requiring code changes to your application. The only requirement is that your application must enable the `trusted_schema` pragma in SQLite:

```sql
PRAGMA trusted_schema = ON;
```

### What happens if a node goes offline?

If a node goes offline:
1. Other nodes continue operating normally
2. Changes are queued in NATS JetStream
3. When the node reconnects, it processes missed changes
4. If offline for an extended period, it may restore from a snapshot first

### How are conflicts handled?

HarmonyLite uses a "last-writer-wins" conflict resolution strategy:
1. Each change has a timestamp and node ID
2. If two nodes modify the same row, the latest change (by timestamp) is kept
3. If timestamps are identical, the node with the higher ID wins

Custom conflict resolution is not currently supported.

### What version of SQLite is required?

HarmonyLite requires SQLite version 3.35.0 or newer. This version introduced the `RETURNING` clause which is used by HarmonyLite's triggers.

### How much overhead does HarmonyLite add?

The overhead varies depending on your workload:
- **Storage**: Change log tables add 20-100% overhead depending on change frequency
- **CPU**: Minimal impact for read operations, 5-15% for write operations
- **Latency**: Local operations typically add 1-5ms overhead
- **Replication Delay**: Usually 50-500ms depending on network conditions

## Deployment Questions

### Can I use HarmonyLite with containerized applications?

Yes, HarmonyLite works well in containerized environments. You can:
1. Run HarmonyLite in its own container with a shared volume for the SQLite database
2. Use a sidecar container pattern alongside your application container
3. Deploy with Kubernetes using StatefulSets for stable node identities

### Can I deploy HarmonyLite across multiple regions?

Yes, HarmonyLite can work across regions with higher latency connections. For multi-region deployments:
1. Configure NATS with gateways between regions
2. Increase `max_entries` and `replicas` for better fault tolerance
3. Enable compression to reduce bandwidth usage
4. Expect higher replication latency due to network distance

### Do I need an external NATS server?

No, HarmonyLite can run with its embedded NATS server for simplicity:
- For small deployments (3-5 nodes), the embedded server works well
- For larger deployments, a dedicated NATS cluster is recommended
- For production, external NATS provides better monitoring and management

### How do I back up a HarmonyLite database?

HarmonyLite provides several backup options:
1. **Automatic snapshots**: Configure the `[snapshot]` section in your config
2. **Regular SQLite backups**: Standard SQLite backup procedures work normally
3. **Filesystem backups**: Back up the entire data directory
4. **Node replication**: Additional nodes can serve as live backups

### Can I use HarmonyLite with a read-only replica design?

Yes, HarmonyLite supports read-only replica configurations:
- Set `publish=false` on nodes designated as read-only replicas
- These nodes will receive changes but not publish their own
- This can be useful for creating dedicated reporting or analytics nodes

## Performance Questions

### How many transactions per second can HarmonyLite handle?

Performance depends on many factors, but general guidelines:
- **Single node write throughput**: 1,000-5,000 TPS
- **Cluster write throughput**: Similar to single node (eventual consistency)
- **Read throughput**: Scales linearly with number of nodes

These numbers can vary significantly based on hardware, network, and workload patterns.

### How does HarmonyLite perform with large databases?

HarmonyLite works with databases ranging from megabytes to many gigabytes:
- For large databases (10GB+), consider:
  - More frequent snapshots
  - Higher `cleanup_interval` values
  - SSD storage for better performance
  - Tuning SQLite's cache size in your application

### How do I optimize HarmonyLite for my workload?

For read-heavy workloads:
- Add more nodes to distribute reads
- Use appropriate SQLite indexes
- Consider memory tuning for SQLite in your application

For write-heavy workloads:
- Increase `replication_log.shards` for parallel processing
- Enable compression if network is a bottleneck
- Consider SSD storage for better performance
- Tune the `cleanup_interval` for optimal change log maintenance

### Does HarmonyLite support sharding?

HarmonyLite internally shards change streams for better performance, but doesn't provide application-level sharding. For very large datasets:
1. Consider application-level sharding with multiple HarmonyLite clusters
2. Each cluster manages a subset of your data
3. Your application directs operations to the appropriate cluster

## Troubleshooting Questions

### Why aren't my changes replicating?

Common reasons for replication issues:
1. **NATS connectivity problems**: Check network and firewall settings
2. **Triggers not installed**: Run `harmonylite -cleanup` and restart
3. **PRAGMA trusted_schema not enabled**: Ensure your app sets this
4. **Node configuration issues**: Verify `publish` and `replicate` settings
5. **Database permissions**: Check file permissions and ownership

See the [Troubleshooting Guide](troubleshooting.md) for detailed diagnostics.

### I'm getting "database is locked" errors. What should I do?

SQLite locking issues can occur for several reasons:
1. **Application transaction handling**: Ensure transactions are properly closed
2. **Journal mode**: Use WAL mode in your application
3. **Busy timeout**: Set an appropriate timeout in your application
4. **Multiple processes**: Check if multiple processes are accessing the database
5. **File system issues**: Verify proper file system permissions and mount options

### How do I recover from database corruption?

If you suspect database corruption:
1. Stop your application and HarmonyLite
2. Run `PRAGMA integrity_check;` on the database
3. If corruption is confirmed, restore from the latest snapshot:
   - Remove the database file and sequence map
   - Restart HarmonyLite to trigger automatic recovery
4. If snapshots aren't available, restore from a backup
5. As a last resort, dump and recreate the schema and data

### What if I need to make schema changes?

For schema changes:
1. Stop applications writing to the database
2. Apply schema changes on one node
3. Run cleanup to reset triggers:
   ```bash
   harmonylite -config /path/to/config.toml -cleanup
   ```
4. Restart HarmonyLite on that node
5. Wait for changes to replicate
6. Repeat for other nodes
7. Resume application connections

## Advanced Questions

### Can I use HarmonyLite with encrypted SQLite databases?

Yes, HarmonyLite works with encrypted SQLite databases. If using SQLCipher or similar encryption:
1. Your application needs to handle key management and decryption
2. HarmonyLite needs the same key to access the database
3. Snapshots will be encrypted with the same encryption

### Is HarmonyLite suitable for IoT or edge computing?

Yes, HarmonyLite is well-suited for IoT and edge computing:
- Lightweight enough to run on constrained devices
- Works with intermittent connectivity
- Eventual consistency model tolerates network disruptions
- Enables local writes with later synchronization
- Configurable storage and memory usage

### Can I integrate HarmonyLite with my existing monitoring system?

Yes, HarmonyLite provides several monitoring integration points:
- **Prometheus metrics**: Enable with the `[prometheus]` config section
- **Structured logging**: Use JSON logging for log aggregation systems
- **NATS monitoring**: Monitor the underlying NATS infrastructure
- **Health checks**: Use the metrics endpoint for service health checks

### Is there a cloud-hosted version of HarmonyLite?

Currently, HarmonyLite is self-hosted software that you deploy and manage. There is no official cloud-hosted service offering HarmonyLite as a managed service.

### How do I contribute to HarmonyLite?

Contributions are welcome! To contribute:
1. Fork the repository on GitHub
2. Create a feature branch for your changes
3. Make your changes, including tests
4. Submit a pull request
5. Engage with the community for feedback

## Support Questions

### Where can I get help with HarmonyLite?

Support resources include:
- **Documentation**: Comprehensive guides and references
- **GitHub Issues**: For bug reports and feature requests
- **Community Forums**: Discuss with other users and developers
- **Stack Overflow**: Tag questions with 'harmonylite'

### Is commercial support available?

For commercial support inquiries, please contact the maintainers directly through GitHub.

### How do I report a bug?

To report a bug:
1. Check existing GitHub issues to see if it's already reported
2. Create a new issue with:
   - Clear description of the problem
   - Steps to reproduce
   - Expected vs. actual behavior
   - Version information
   - Logs and configuration (redacted of sensitive data)

### How often is HarmonyLite updated?

HarmonyLite follows a regular release cycle with:
- **Patch releases**: Bug fixes and minor improvements
- **Minor releases**: New features and enhancements
- **Major releases**: Significant changes that may require configuration updates

Check the GitHub repository for the latest release information.