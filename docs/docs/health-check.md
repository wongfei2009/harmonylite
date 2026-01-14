---
id: health-check
title: Health Check Endpoint
sidebar_label: Health Check
description: Configure and use the health check endpoint for monitoring HarmonyLite nodes
---

# Health Check Endpoint

HarmonyLite includes a health check HTTP endpoint that can be used to monitor the status of nodes in your cluster. This is particularly useful when running HarmonyLite in containerized environments like Docker or Kubernetes.

:::tip
The health check endpoint is disabled by default. You must explicitly enable it in your configuration.
:::

## Configuration

You can configure the health check endpoint in your `config.toml` file:

```toml
[health_check]
# Enable/disable the health check endpoint
enable = false  # Disabled by default
# HTTP endpoint to expose for health checks
bind = "0.0.0.0:8090"
# Path for the health check endpoint
path = "/health"
# Detailed response with metrics (if false, only returns status code)
detailed = true
```

## Usage

When enabled, the health check endpoint provides information about the node's status. The endpoint returns:

- HTTP 200 OK: When the node is healthy
- HTTP 503 Service Unavailable: When the node is unhealthy or in an error state

### Detailed Mode

When `detailed` is set to `true` (default), the health check response will include a JSON body with detailed information:

```json
{
  "status": "healthy",
  "node_id": 1,
  "uptime_seconds": 3600,
  "db_connected": true,
  "nats_connected": true,
  "cdc_installed": true,
  "tables_tracked": 5,
  "last_replicated_event_timestamp": "2025-03-21T15:30:45Z",
  "last_published_event_timestamp": "2025-03-21T15:30:40Z",
  "version": "1.0.0"
}
```

When `detailed` is set to `false`, only the HTTP status code is returned, which is useful for lightweight health checks.

## Docker Integration

When using Docker, you can configure the health check in your `docker-compose.yml` file:

```yaml
services:
  harmonylite:
    image: harmonylite:latest
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8090/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

## Kubernetes Integration

For Kubernetes, you can configure liveness and readiness probes:

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8090
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /health
    port: 8090
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Custom Health Checks

The health check endpoint checks the following components:

1. Database connectivity
2. NATS connection status
3. CDC (Change Data Capture) hooks installation
4. Tables being tracked

If any of these checks fail, the node will be considered unhealthy.
