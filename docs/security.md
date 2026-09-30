# Security model

An authorized Compose publisher can administer the Docker host. Docker socket access,
privileged containers, bind mounts, host namespaces and external resources remain
powerful. This agent is not a sandbox for untrusted customers. OS service hardening
reduces accidental access but does not remove Docker's host-level authority.

Use TLS with certificate verification, per-node broker accounts and exact-topic ACLs:
agents may read their commands and write their events; controllers have the inverse
rights. Do not grant wildcard access or anonymous production access. Protect controller
credentials like host administrator credentials. Restrict network access to the broker.

Broker requirements:

- MQTT 3.1.1 persistent sessions and persistent storage on durable disk.
- Maximum packet size 1,600,000 bytes and bounded per-client queued messages.
- Disallow retained command publication (for Mosquitto: `retain_available false` on
  a dedicated broker). MQTT may deliver a live retained publication with the retained
  flag cleared; application rejection of retained replays alone cannot enforce this.
- Monitor authentication failures, topic denials, queue pressure and disk usage.

The sample broker under `docker/mosquitto` is for localhost development only. Production
broker TLS certificates, ACLs and backup policy are infrastructure responsibilities.

State contains plaintext Compose configuration and pending payloads, which may contain
workload secrets. Protect state with filesystem permissions, encrypted storage and
restricted backups. Completed receipts discard payloads. Secrets are not echoed in
Compose failure diagnostics. Never commit production config, keys or credentials.

Release workflows run tests before packaging, pin third-party actions by commit, and
create checksums and GitHub build attestations. The `release` environment is limited
to version tags and existing release tags cannot be changed or deleted. Publication
follows artifact verification by the authorized release operator; teams can additionally
configure required environment reviewers when separate human approval is needed. Pull request
workflows use read-only permissions and never use `pull_request_target`.

Report suspected vulnerabilities privately using the repository's GitHub security
advisory feature when enabled. Do not include credentials or exploit payloads containing
private data in public issues. See `SECURITY.md`.
