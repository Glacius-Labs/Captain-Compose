# Monitoring and incident diagnosis

Set `monitor_listen: 127.0.0.1:9080` to enable the read-only local HTTP listener.
Only numeric loopback bindings are accepted. For central scraping, run a collector
on the host or an authenticated TLS reverse proxy. Do not expose a raw Docker socket
or proxy the listener publicly without access control.

| Endpoint | Meaning |
| --- | --- |
| `GET /healthz` | Agent HTTP loop is alive; not application readiness |
| `GET /readyz` | MQTT connected and a successful Docker observation younger than one minute |
| `GET /status` | Timestamped safe deployment observations and queue state |
| `GET /metrics` | Prometheus gauges with bounded cardinality; no request IDs or payloads |

Observations run every 15 seconds with a ten-second deadline. A failed observation
clears Docker readiness and leaves the previous timestamp visible. Container health
does not prove application business correctness. Supply meaningful Compose healthchecks.

Suggested starting alerts (adjust against measured workload behavior):

- Agent scrape unavailable or broker disconnected for two minutes.
- Docker observation unavailable/stale for one minute.
- Oldest event exceeds two minutes, or pending events approach the documented capacity.
- Pending commands age beyond the normal rollout duration.
- Any drifted deployment or unhealthy service persists beyond its startup allowance.
- Host disk space below the operational reserve, certificate expiry, repeated restarts.

The host collector must separately monitor filesystem capacity and certificate expiry;
agent gauges do not claim to provide those measurements. Examples are in
`deploy/monitoring/prometheus-alerts.yml`.

Optional `status_topic` enables a non-retained QoS 1 heartbeat every 30 seconds and
an MQTT last will with `online:false`. Grant the agent write access and observers read
access to that exact topic. Heartbeats include version, queue counts and observation
timestamps, never Compose manifests. Expire an agent's online status after 90 seconds
without a heartbeat: a last will is helpful but not a complete failure detector.
Normal service shutdown can stop heartbeats without a last will. Broker configurations
that disable retained publication remain compatible.

For incidents, start with `captain-compose doctor`, then request status/inspect and
query the original request ID. Preserve that ID and the exact request when delivery
is uncertain. Inspect journald and Docker container state locally with appropriately
privileged access. Do not include workload environment variables or manifests in
public bug reports. Raw Docker stderr remains restricted because it may contain secrets.
