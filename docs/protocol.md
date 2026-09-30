# MQTT protocol

Use MQTT 3.1.1, authenticated TLS and per-node exact topics. Publish commands at
QoS 1 with retain disabled. Subscribe to events before submitting commands.
Malformed, oversized, retained replay and QoS 0 messages are rejected and logged;
they are acknowledged to prevent an infinite poison-message replay loop. Accepted
requests are durably journaled before acknowledgement.

The envelope `id` is a UUID and identifies the request. Version omitted or `1`
selects the original create/remove contract. Version `2` enables named operations,
expiry, revision guards, queryable results and machine-readable events. Unknown
fields and trailing JSON values are rejected. Full messages are limited to 1,536,000
bytes; create payloads are standard base64 and decode to at most 1 MiB of YAML.

## Version 1

Existing create and remove messages remain supported:

```json
{"id":"673fca08-5e71-45f9-bfe3-4cb9b79c4731","type":"create","data":{"name":"web","payload":"BASE64_ENCODED_COMPOSE_YAML"}}
```

```json
{"id":"f6a01bfe-07b7-44f9-8a9b-f1a7b52f3a81","type":"remove","data":{"name":"web"}}
```

Removal preserves volumes. Data disposal is an explicit operator task outside this
protocol. Legacy requests keep their original event shape and completed-ID behavior.

## Version 2

Version 2 accepts `create`, `remove`, `plan`, `status`, `inspect`, `doctor`, `revert`
and `result`. `data` is action-specific: create and plan need `name` and `payload`;
remove needs `name`; status accepts an optional `name`; inspect needs `name`; doctor
takes an empty object; result needs `request_id`; revert needs `name`,
`expected_revision`, `revision` and `allow_data_risk: true`. Create and plan may use
an empty expected revision to request create-only behavior. Non-empty expected
revisions, remove guards and both revert revisions must be lowercase 64-character
SHA-256 values. Callers cannot supply the internal request ID.

```json
{
  "id": "673fca08-5e71-45f9-bfe3-4cb9b79c4731",
  "version": 2,
  "type": "create",
  "expires_at": "2026-09-30T12:15:00Z",
  "data": {
    "name": "web",
    "payload": "BASE64_ENCODED_COMPOSE_YAML",
    "expected_revision": ""
  }
}
```

`expires_at` is an optional UTC RFC 3339 timestamp on mutations; query requests omit
it. It is checked immediately before the first mutation. The journal records when
execution starts, so a restart does not misreport a command that may already have begun as expired. An empty
`expected_revision` requests create-only behavior; a non-empty value must be a
lowercase 64-character SHA-256 revision. The runtime enforces the optimistic revision
check. Revert requires both a revision and `allow_data_risk: true`; it restores
configuration and does not roll application data back.

Queries use the same command topic. A result lookup has its own request ID and puts
the original request ID in `data.request_id`:

```json
{"id":"e5bda20f-0138-4ee3-a8b2-94584978492b","version":2,"type":"result","data":{"request_id":"673fca08-5e71-45f9-bfe3-4cb9b79c4731"}}
```

Its event reports `pending` (not executed yet), `executed` (result is journaled but
delivery is unconfirmed), `delivered` (broker acknowledged the saved result), or
`unknown` (no retained receipt). An available saved event is returned as `data.result`.
Identical retries of a completed v2 mutation republish its saved result. Reusing an ID
with different content is rejected. Mutation receipts are retained for seven days,
up to 10,000 receipts. Query receipts are retained for ten minutes, up to 1,000
receipts, so polling does not consume the mutation receipt budget. Pending plus
undelivered work is bounded at 128 entries (112 mutation slots and 16 query slots);
when full, the incoming command remains unacknowledged. The listener keeps up to 16
such incoming messages in a bounded
in-memory retry queue and acknowledges each only after it is durably journaled.
Further messages remain unacknowledged in the broker until queue space frees. The
journal never drops accepted work. Keep query polling below the query receipt limit;
a saturated query quota uses the same bounded retry path.

## Events

Version 1 events preserve their existing `action` (`create` or `delete`) and fields.
Version 2 events add `version: 2`, a stable machine-readable `code`, and optional
sanitized `data`. The v2 Runtime contract returns operator-safe query results; the
adapter does not copy submitted request data or raw runtime errors into events. Events
retain `id`, `request_id`, `timestamp`, `action`, `name`,
`success`, `message`, and `labels` for existing consumers. Version 2 events are
bounded to 256 KiB. An oversized query returns `response_too_large` without data and
suggests narrowing the query. Event publication is QoS 1, non-retained and retried
independently of command execution.

## Migration from the prototype

Unreleased development snapshots with journal records lacking `sequence` are not
compatible with the stable journal format. Do not discard pending commands. Drain
them with the matching development binary, stop the agent, back up the complete
state, and start with a fresh journal only when no pending work remains. Preserve
deployment manifests and controller-side request history during this transition.

Requests now require `id`; use a new UUID for each intended operation. Move shared
topics to per-node topics and client IDs. Plaintext connections require the explicit
development setting `allow_insecure: true`; disabling TLS verification is rejected.
TLS is selected by the secure broker URL scheme (`ssl://`, `tls://` or `wss://`);
the redundant prototype `mqtt.tls.enable` option is no longer accepted. Configure
`mqtt.tls.ca_cert_path` and optional client certificate/key paths as needed.
The old prototype deleted manifests and used unprefixed Docker projects, so it
cannot automatically transfer ownership. Export and recover old manifests, back up
data, stop the old agent, and migrate each workload explicitly. Do not run old and
new agents on the same command topic. Remove old workloads manually only after
verifying the new deployment and volume mapping. There is no implicit import or
volume rename.
