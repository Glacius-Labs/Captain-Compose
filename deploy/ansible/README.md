# Ansible deployment

This role installs and operates Captain Compose on a dedicated Ubuntu 24.04-or-newer
Docker host. It requires Docker Engine, the Docker Compose plugin at least 2.20.0,
systemd, Python 3, and GitHub CLI (`gh`) on the target. The Compose socket grants
host-administration capability; only use the role on a host dedicated to trusted
publishers. CI validates the role with the pinned `ansible-core` in
`requirements.txt`; the inventory runner's controller Python must meet that version's
requirements.

Set `captain_compose_version` to an explicit published release and provide
`captain_compose_config_source` on the Ansible controller. Example inventory:

```ini
[captain_compose]
agent-01 ansible_host=192.0.2.10
```

Example variables (store secrets with Ansible Vault):

```yaml
captain_compose_version: 1.1.0-rc.1
captain_compose_config_source: ./secrets/agent-01.yaml
captain_compose_start_service: true
# Optional. Use a dedicated Docker config, never root's home configuration.
captain_compose_docker_config_source: ./secrets/agent-01-docker.json
captain_compose_managed_secret_files:
  - src: ./secrets/agent-01-password
    dest: mqtt-password
  - src: ./secrets/agent-01-ca.pem
    dest: ca.pem
```

Validate syntax, then apply:

```bash
ansible-playbook --syntax-check deploy/ansible/site.yml
ansible-playbook -i inventory.ini deploy/ansible/site.yml
```

The role downloads a pinned version's Linux archive and checksum manifest, validates
the checksum, then verifies the GitHub artifact attestation against the exact repository,
release workflow, and version tag. It rejects self-hosted-runner attestations. Offline
verification is supported by setting both
`captain_compose_offline_attestation_bundle` and
`captain_compose_offline_trusted_root` to files already available on the target.
Checksum-only install is disabled by default; `captain_compose_verify_provenance: false`
is an explicit exception for controlled bootstrap/test environments only.

The role preserves an existing `config.yaml` and makes it `root:captain-compose`
mode `0640`. A supplied Docker config is copied to `/etc/captain-compose/docker` with
the same owner and mode, and the unit points `DOCKER_CONFIG` there. Additional MQTT
password/TLS files can be listed with relative `dest` paths under the config directory.
Secret copy and preflight tasks suppress logs. On each apply, the service stops before
reloading changed binary/config content; it starts again only after the new binary passes its configuration and Docker
preflight. A failed preflight leaves it stopped for diagnosis. Existing workloads are
not removed or rolled back.

Use the backup and restore procedures in [the lifecycle guide](../../docs/lifecycle.md)
before changing versions or restoring a node. For MQTT and registry key rotation see
[credential rotation](../../docs/credential-rotation.md).
