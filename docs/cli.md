# Operator CLI

Build and run the controller as `captain-compose`; the agent remains the separate
`captain-compose-mqtt` service. Configure one or more named environments in
`config/operator.example.yaml`, then select one with `--environment`. Each
environment has one broker URL and the exact command and event topics for one agent.
TLS URLs verify the server certificate and hostname by default. Set `tls.ca_cert_path`
for a private CA and optionally set both client certificate paths for mutual TLS.
Use `password_file` or `CAPTAIN_COMPOSE_MQTT_PASSWORD`; inline passwords and command
line password flags are not supported. The environment variable takes precedence.
Plain MQTT requires the explicit development-only `allow_insecure: true` setting.

```sh
captain-compose --config operator.yaml --environment production deploy web compose.yaml
captain-compose --config operator.yaml --environment production plan web compose.yaml
captain-compose --config operator.yaml --environment production status
captain-compose --config operator.yaml --environment production inspect web
captain-compose --config operator.yaml --environment production remove web
captain-compose --config operator.yaml --environment production doctor
captain-compose --config operator.yaml --environment production result 673fca08-5e71-45f9-bfe3-4cb9b79c4731
captain-compose --config operator.yaml --environment production revert web --revision REVISION --expected-revision CURRENT_REVISION --allow-data-risk
```

`deploy` and `remove` wait for an agent result by default. Add `--wait=false` to return
after the broker acknowledges publication; that only proves broker acceptance, not
agent acceptance or execution. The printed request ID can be used with `result` or
`wait` if delivery is uncertain. Both commands use fresh result-query IDs. `result`
performs one lookup and reports `pending` if work is still in progress; `wait` polls
until the saved operation result is available or the environment timeout expires.
Result queries never repeat the mutation. A timed-out mutation prints its request ID;
retry it with `--request-id` and the same operation content, or keep the exact envelope
in a `--request-file` for replay. Reusing an ID with different content is rejected by
the agent. `--retry-interval` controls the bounded retry interval, and `--expires`
sets a mutation's RFC3339 expiry from the current time. If you supply `--request-id`
for a first attempt, expiry is omitted unless `--expires` is also supplied, so the
same command content and ID can be retried unchanged. If the CLI generated the ID and
expiry, keep its `--request-file`; it records the exact envelope and its named
environment, broker URL, and topics, and rejects mismatched action or input reuse.
The CLI syncs the record and its parent directory before publishing; Windows only
supports syncing the file through Go's portable file API.
Result lookups also return completed version 1 create/delete events for compatibility
with the existing journal format.

`plan` and `deploy` accept a non-empty lowercase SHA-256 `--expected-revision` for
compare-and-swap updates. Use `--create-only` or an explicitly empty
`--expected-revision` on deploy/plan to fail when a deployment already exists. Empty
expected revisions are rejected for remove and revert. `revert`
requires a current `--expected-revision` precondition as well as a target `--revision`
and `--allow-data-risk`; it restores a previous Compose revision but does not restore
application data. `status` returns the agent's
deployment list; `status NAME` filters it to one deployment. `inspect NAME` returns
one deployment's observed state. `doctor`
performs an authenticated request/response round trip and reports the checks available
from that agent; a successful round trip proves access to the exact configured topics,
not every possible broker ACL.

Pass global `--json` for automation. Exit codes are stable: 0 means the requested
result succeeded (or, with `--wait=false`, the broker accepted the request); 1 means
local configuration, runtime, or broker transport failure; 2 means invalid usage; 3
means the operation result is unknown, pending, or the wait expired; and 4 means the agent
reported a remote operation failure. On an uncertain result, keep and reuse its request
ID rather than inventing a new one.

Run `captain-compose --version` to print build metadata.
