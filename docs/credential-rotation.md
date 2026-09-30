# Credential rotation

Rotate one identity or credential set at a time and keep access scoped to one node.
Do not put secrets in YAML output, command-line arguments, Ansible facts, journals,
CI logs or shell history. The service account reads `/etc/captain-compose` through the
`captain-compose` group; the directory and files should remain root-owned and mode
`0750`/`0640`.

## MQTT password or certificate

1. Create a replacement broker credential for this node while the old one remains
   valid. Keep its exact-topic read/write ACLs unchanged.
2. Install the replacement password file or TLS certificate/key under
   `/etc/captain-compose` as `root:captain-compose`, mode `0640`. The CA bundle and
   private key must be regular files, not symlinks. If using `password_file`, remove
   any old inline `password` value; the configuration rejects both together.
3. Update the file path only when paths change, then restart the service during a
   controlled broker maintenance window:

   ```bash
   sudo systemctl restart captain-compose.service
   sudo systemctl is-active captain-compose.service
   sudo journalctl -u captain-compose.service --since -5m
   ```

4. Confirm the node reconnects and completes a safe query/health observation. Revoke
   the old broker credential only after validating the new one.

Do not change the MQTT client ID during routine rotation: a stable ID retains the
broker session and its queued commands/events. If the ID must change, treat it as a
node migration and reconcile the old session first.

## Docker registry credentials

The systemd unit sets `DOCKER_CONFIG=/etc/captain-compose/docker`. Root's
`~/.docker/config.json` is never copied or read automatically. To provide registry
credentials explicitly, stage a Docker client config file outside the repository with
restricted access and pass it to provisioning:

```bash
sudo bash scripts/provision.sh --config ./production.yaml \
  --docker-config-source /secure/staging/config.json
```

The command copies it as `root:captain-compose`, mode `0640`. Replace the credential
using the same explicit source option and restart the service after checking the new
registry token. Avoid credential-helper settings unless the helper is installed at a
fixed system path and can run as the `captain-compose` service user. Do not run
`docker login` as root and assume the service will inherit those credentials.

## Backup and revocation

Credential files are included in the encrypted offline backup. A backup is itself
secret material. Restrict and audit restore access; after a credential is revoked,
assume older backups still contain it. Rotate it again if an old backup is exposed.
