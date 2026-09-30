# MQTT protocol

Use MQTT 3.1.1, authenticated TLS and per-node exact topics. Publish commands at
QoS 1 with retain disabled. Subscribe to events **before** submitting commands.
Malformed, oversized, retained replay and QoS 0 messages are rejected and logged;
they are acknowledged to prevent an infinite poison-message replay loop. They do
not produce deployment events. Valid commands produce success or failure events.

## Create or update

```json
{
  "id": "673fca08-5e71-45f9-bfe3-4cb9b79c4731",
  "type": "create",
  "data": {
    "name": "web",
    "payload": "BASE64_ENCODED_COMPOSE_YAML"
  }
}
```

`payload` is standard base64, decoded to at most 1 MiB of YAML. The full JSON message
is limited to 1,536,000 bytes. Configure the broker packet limit to 1,600,000 bytes;
the MQTT client library allocates the incoming packet before application validation.
Unknown JSON fields and multiple JSON objects are rejected.

Generate a request without hand-encoding base64 (Python 3):

```bash
python3 -c 'import base64,json,uuid; print(json.dumps({"id":str(uuid.uuid4()),"type":"create","data":{"name":"web","payload":base64.b64encode(open("compose.yaml","rb").read()).decode()}}))' > create.json
mosquitto_pub -h localhost -t captain-compose/dev-node/commands -q 1 -f create.json
```

This local example assumes the development broker. For production, use TLS and your
broker's credential handling; never commit credential files or pass passwords in
shared scripts. Keep the exact request file and ID when retrying uncertain delivery.

## Remove

```json
{
  "id": "f6a01bfe-07b7-44f9-8a9b-f1a7b52f3a81",
  "type": "remove",
  "data": { "name": "web" }
}
```

Removal preserves volumes. Data disposal is an explicit operator task outside this
protocol. Reusing a completed ID does not republish a new result; keep a durable
event subscription and retain events on the controller side.

## Events

```json
{
  "id": "1ab3b4cf-cb57-49a0-9ba8-8c2217534a48",
  "request_id": "f6a01bfe-07b7-44f9-8a9b-f1a7b52f3a81",
  "timestamp": "2026-09-30T12:00:00Z",
  "action": "delete",
  "name": "web",
  "success": true,
  "message": "Deployment \"web\" removed successfully",
  "labels": null
}
```

`action` remains `create` or `delete` for compatibility with existing event consumers.
Failure events include an `error` label. Result publication is QoS 1, non-retained.

## Migration from the prototype

Requests now require `id`; use a new UUID for each intended operation. Move shared
topics to per-node topics and client IDs. Plaintext connections require the explicit
development setting `allow_insecure: true`; disabling TLS verification is rejected.
The old prototype deleted its manifests and used unprefixed Docker projects, so it
cannot automatically transfer ownership. Export/recover old manifests, back up data,
stop the old agent, and migrate each workload explicitly. Do not run old and new
agents on the same command topic. Remove old workloads manually only after verifying
the new deployment and volume mapping. There is no implicit import or volume rename.
