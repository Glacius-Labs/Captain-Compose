#!/usr/bin/env bash
set -Eeuo pipefail
[[ "${PILOT_TRACE:-}" != 1 ]] || set -x

usage() {
  echo 'Usage: scripts/pilot/run.sh [--duration 2m] [--report PATH]'
}

DURATION_TEXT=2m
REPORT_PATH=''
while (($#)); do
  case "$1" in
    --duration) (($# >= 2)) || { usage >&2; exit 2; }; DURATION_TEXT=$2; shift 2 ;;
    --report) (($# >= 2)) || { usage >&2; exit 2; }; REPORT_PATH=$2; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

if [[ "${CAPTAIN_PILOT_DIND:-}" != 1 ]]; then
  echo 'Refusing to run: start scripts/pilot/run.sh inside the documented privileged docker:dind runner.' >&2
  exit 2
fi
[[ -S /var/run/docker.sock ]] || { echo 'The disposable runner Docker socket is unavailable.' >&2; exit 2; }
if [[ "$DURATION_TEXT" =~ ^([1-9][0-9]*)(s|m|h|d)$ ]]; then
  DURATION_VALUE=${BASH_REMATCH[1]}
  case "${BASH_REMATCH[2]}" in s) DURATION_SECONDS=$DURATION_VALUE ;; m) DURATION_SECONDS=$((DURATION_VALUE * 60)) ;; h) DURATION_SECONDS=$((DURATION_VALUE * 3600)) ;; d) DURATION_SECONDS=$((DURATION_VALUE * 86400)) ;; esac
else
  echo 'Duration must be a positive integer followed by s, m, h, or d.' >&2
  exit 2
fi

START_UTC=$(date -u +%Y-%m-%dT%H:%M:%SZ)
START_SECONDS=$(date -u +%s)
SOURCE_SHA=$(git -C "$PWD" rev-parse HEAD)
if [[ -n "$(git -C "$PWD" status --porcelain --untracked-files=normal)" ]]; then SOURCE_DIRTY=true; else SOURCE_DIRTY=false; fi
GO_VERSION=$(GOTOOLCHAIN=auto go version | awk '{print $3}')
LEGACY_TAG_SHA=$(git -C "$PWD" rev-parse 'v1.0.0^{commit}')
DOCKER_CLIENT_VERSION=$(docker version --format '{{.Client.Version}}')
DOCKER_SERVER_VERSION=$(docker version --format '{{.Server.Version}}')
COMPOSE_VERSION=$(docker compose version --short)
MOSQUITTO_IMAGE=eclipse-mosquitto:2.0.22
ARCHITECTURE=$(uname -m)
V1_AGENT_SHA256=''
AGENT_SHA256=''
OPERATOR_SHA256=''
PILOT_ID="pilot-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
WORK=$(mktemp -d "/tmp/${PILOT_ID}.XXXXXX")
BIN_DIR="$WORK/bin"
STATE_DIR="$WORK/state"
CONFIG_DIR="$WORK/config"
mkdir -p "$BIN_DIR" "$STATE_DIR" "$CONFIG_DIR" "$WORK/certs" "$WORK/v1"
REPORT_PATH=${REPORT_PATH:-"$PWD/scripts/pilot/pilot-report-${PILOT_ID}.json"}
[[ "$REPORT_PATH" != "$WORK"* ]] || { echo '--report must be outside the disposable /tmp directory.' >&2; exit 2; }
COMMAND_TOPIC="captain-compose/$PILOT_ID/commands"
EVENT_TOPIC="captain-compose/$PILOT_ID/events"
BROKER="${PILOT_ID}-broker"
USERNAME=pilot
PASSWORD="pilot-${RANDOM}-${RANDOM}"
AGENT_PID=''
V1_PID=''
DISK_MOUNT=''
CHECKS='[]'
ACTIONS='[]'
FAILURES='[]'
NOT_RUN=$(jq -cn --arg arch "$ARCHITECTURE" '["physical_reboot", "sudden_power_loss", "systemd_host_lifecycle"] + (if $arch=="aarch64" or $arch=="arm64" then [] else ["native_arm64"] end)')
MONITOR_SAMPLES=0
LAST_SAMPLE_SECONDS=0
MAX_SAMPLE_GAP_SECONDS=0
MAX_SAMPLE_GAP_LIMIT=120
EXIT_STATUS=0
FINALIZED=0
OBSERVATION_START_UTC=''
OBSERVATION_START_SECONDS=0
CURRENT_STAGE=runner_setup

write_report() {
  local now elapsed setup_elapsed observation_elapsed outcome ended
  now=$(date -u +%s)
  elapsed=$((now - START_SECONDS))
  if (( OBSERVATION_START_SECONDS > 0 )); then
    setup_elapsed=$((OBSERVATION_START_SECONDS - START_SECONDS))
    observation_elapsed=$((now - OBSERVATION_START_SECONDS))
  else
    setup_elapsed=$elapsed
    observation_elapsed=0
  fi
  outcome=running
  if (( FINALIZED == 1 )); then
    outcome=passed
    [[ $(jq 'length' <<<"$FAILURES") == 0 && $EXIT_STATUS == 0 ]] || outcome=failed
  fi
  ended=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  mkdir -p "$(dirname "$REPORT_PATH")"
  jq -n --arg pilot_id "$PILOT_ID" --arg outcome "$outcome" --arg started_at "$START_UTC" --arg updated_at "$ended" \
    --arg observation_started_at "$OBSERVATION_START_UTC" --arg duration_requested "$DURATION_TEXT" \
    --arg source_sha "$SOURCE_SHA" --arg architecture "$ARCHITECTURE" --arg legacy_tag_sha "$LEGACY_TAG_SHA" --arg go_version "$GO_VERSION" --arg docker_client_version "$DOCKER_CLIENT_VERSION" \
    --arg v1_agent_sha256 "$V1_AGENT_SHA256" --arg agent_sha256 "$AGENT_SHA256" --arg operator_sha256 "$OPERATOR_SHA256" \
    --arg docker_server_version "$DOCKER_SERVER_VERSION" --arg compose_version "$COMPOSE_VERSION" --arg mosquitto_image "$MOSQUITTO_IMAGE" \
    --argjson source_dirty "$SOURCE_DIRTY" \
    --arg report_scope "disposable_nested_docker" --argjson elapsed_seconds "$elapsed" \
    --argjson setup_elapsed_seconds "$setup_elapsed" --argjson observation_elapsed_seconds "$observation_elapsed" \
    --argjson monitor_samples "$MONITOR_SAMPLES" --argjson max_sample_gap_seconds "$MAX_SAMPLE_GAP_SECONDS" --argjson max_sample_gap_limit_seconds "$MAX_SAMPLE_GAP_LIMIT" --argjson checks "$CHECKS" --argjson observed_actions "$ACTIONS" \
    --argjson failures "$FAILURES" --argjson not_run "$NOT_RUN" \
    '{pilot_id:$pilot_id,outcome:$outcome,started_at:$started_at,updated_at:$updated_at,observation_started_at:(if $observation_started_at=="" then null else $observation_started_at end),elapsed_seconds:$elapsed_seconds,setup_elapsed_seconds:$setup_elapsed_seconds,observation_elapsed_seconds:$observation_elapsed_seconds,duration_requested:$duration_requested,report_scope:$report_scope,source:{sha:$source_sha,dirty:$source_dirty},platform:{architecture:$architecture},tools:{go:$go_version,docker_client:$docker_client_version,docker_server:$docker_server_version,compose:$compose_version,legacy_tag:"v1.0.0",legacy_tag_sha:$legacy_tag_sha,mosquitto_image:$mosquitto_image},binaries:{legacy_agent_sha256:(if $v1_agent_sha256=="" then null else $v1_agent_sha256 end),agent_sha256:(if $agent_sha256=="" then null else $agent_sha256 end),operator_sha256:(if $operator_sha256=="" then null else $operator_sha256 end)},monitor_samples:$monitor_samples,max_sample_gap_seconds:$max_sample_gap_seconds,max_sample_gap_limit_seconds:$max_sample_gap_limit_seconds,checks:$checks,observed_actions:$observed_actions,failures:$failures,not_run:$not_run}' >"$REPORT_PATH"
}

action() {
  ACTIONS=$(jq -c --arg at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg name "$1" --arg detail "$2" '. + [{at:$at,name:$name,detail:$detail}]' <<<"$ACTIONS")
  write_report
}
check() {
  CHECKS=$(jq -c --arg name "$1" --arg status "$2" --arg detail "$3" '. + [{name:$name,status:$status,detail:$detail}]' <<<"$CHECKS")
  if [[ "$2" != passed ]]; then
    FAILURES=$(jq -c --arg name "$1" --arg message "$3" '. + [{name:$name,message:$message}]' <<<"$FAILURES")
  fi
  write_report
}
fail() {
  check "$1" failed "$2"
  echo "Pilot check failed: $1: $2" >&2
  exit 1
}
operator() {
  "$BIN_DIR/captain-compose" --config "$CONFIG_DIR/operator.yaml" --environment pilot --json "$@"
}
wait_for() {
  local seconds=$2; shift 2
  local until=$(( $(date -u +%s) + seconds ))
  until "$@" >/dev/null 2>&1; do
    (( $(date -u +%s) < until )) || return 1
    sleep 0.25
  done
}
# shellcheck disable=SC2317,SC2329 # Invoked by name through wait_for's command dispatch.
project_running() {
  [[ -n "$(docker ps --filter "label=com.docker.compose.project=cc-$1" --filter status=running --format '{{.ID}}' | head -n 1)" ]]
}
project_exists() {
  [[ -n "$(docker ps --all --filter "label=com.docker.compose.project=cc-$1" --format '{{.ID}}' | head -n 1)" ]]
}
journal_recorded() {
  local id=$1 wanted=$2 file
  for file in "$STATE_DIR/journal"/*.json; do
    [[ -f "$file" ]] || continue
    jq -e --arg id "$id" --arg wanted "$wanted" 'select(.id==$id) | if $wanted=="pending" then .command!=null and .event==null else .event!=null end' "$file" >/dev/null 2>&1 && return 0
  done
  return 1
}
# shellcheck disable=SC2317,SC2329 # Invoked by name through wait_for's command dispatch.
wait_agent_ready() { grep -q 'MQTT subscription ready' "$WORK/agent.log"; }
wait_saved_event() { journal_recorded "$1" complete; }
json_success() { jq -e '.ok == true and .state == "completed"' <<<"$1" >/dev/null; }
json_result_success() { jq -e '.ok == true and (.state == "delivered" or .state == "executed") and .result.success == true' <<<"$1" >/dev/null; }
json_active_deployment() {
  jq -e '.ok == true and .state == "completed" and .result.phase == "active" and .result.drift == false and (.result.services | length > 0) and all(.result.services[]; .state == "running" and .health == "healthy")' <<<"$1" >/dev/null
}
# shellcheck disable=SC2317,SC2329 # Invoked by name through wait_for's command dispatch.
compose_waiting() { project_running "$1"; }
operator_env_config() {
  local password_file=$1 ca_file=$2 timeout=${3:-30s}
  cat >"$CONFIG_DIR/operator.yaml" <<EOF
environments:
  pilot:
    broker_url: ssl://127.0.0.1:18884
    command_topic: $COMMAND_TOPIC
    event_topic: $EVENT_TOPIC
    username: $USERNAME
    password_file: $password_file
    tls:
      ca_cert_path: $ca_file
    timeout: $timeout
    retry_interval: 1s
EOF
  chmod 600 "$CONFIG_DIR/operator.yaml"
}
agent_config() {
  local state_dir=$1 broker_url=$2 allow_insecure=$3 ca_file=$4
  cat >"$CONFIG_DIR/agent.yaml" <<EOF
state_dir: $state_dir
operation_timeout: 45s
mqtt:
  broker_url: $broker_url
  client_id: $PILOT_ID-agent
  username: $USERNAME
  password_file: $CONFIG_DIR/password
  allow_insecure: $allow_insecure
  tls:
    ca_cert_path: $ca_file
listener_topic: $COMMAND_TOPIC
publisher_topic: $EVENT_TOPIC
EOF
  chmod 600 "$CONFIG_DIR/agent.yaml"
}
start_agent() {
  "$BIN_DIR/captain-compose-mqtt" --config "$CONFIG_DIR/agent.yaml" >>"$WORK/agent.log" 2>&1 &
  AGENT_PID=$!
  wait_for 'agent MQTT subscription' 20 wait_agent_ready || return 1
}
stop_agent() {
  [[ -n "$AGENT_PID" ]] || return 0
  kill -TERM "$AGENT_PID" 2>/dev/null || true
  wait "$AGENT_PID" 2>/dev/null || true
  AGENT_PID=''
}
result_request_id() { jq -r '.request_id // empty' <<<"$1"; }

# shellcheck disable=SC2317,SC2329 # Registered as an EXIT trap and invoked indirectly by Bash.
cleanup() {
  stop_agent
  if [[ -n "$V1_PID" ]]; then kill -TERM "$V1_PID" 2>/dev/null || true; wait "$V1_PID" 2>/dev/null || true; fi
  if [[ -n "$DISK_MOUNT" ]] && grep -Fq " $DISK_MOUNT " /proc/mounts; then umount "$DISK_MOUNT" || true; fi
  docker rm -f "$BROKER" >/dev/null 2>&1 || true
  # Pilot deployments have unique cc-<pilot-name> project labels. Remove only those
  # owned projects explicitly created below; never run a broad engine prune.
  local name
  for name in pilotdata backloga backlogb expiryprobe diskfull; do
    docker compose --env-file /dev/null --project-name "cc-$name" --file "$PWD/tests/acceptance/fixtures/persistent-data.yaml" down --remove-orphans >/dev/null 2>&1 || true
  done
}
# shellcheck disable=SC2317,SC2329 # Registered as an EXIT trap and invoked indirectly by Bash.
finish() {
  local original=$?
  (( FINALIZED == 0 )) || return "$original"
  FINALIZED=1
  EXIT_STATUS=$original
  if (( original != 0 )) && [[ $(jq 'length' <<<"$FAILURES") == 0 ]]; then
    check "$CURRENT_STAGE" failed "runner exited with status $original before this stage completed"
  fi
  cleanup
  local end_utc
  end_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  write_report
  jq --arg ended_at "$end_utc" '. + {ended_at:$ended_at}' "$REPORT_PATH" >"$REPORT_PATH.tmp"
  mv "$REPORT_PATH.tmp" "$REPORT_PATH"
  printf 'Acceptance report: %s\n' "$REPORT_PATH"
  cat "$REPORT_PATH"
  rm -rf -- "$WORK"
  exit "$original"
}
trap finish EXIT

command -v jq >/dev/null || { echo 'jq is required inside the disposable runner.' >&2; exit 2; }
command -v openssl >/dev/null || { echo 'OpenSSL is required inside the disposable runner.' >&2; exit 2; }
command -v go >/dev/null || { echo 'Go is required inside the disposable runner.' >&2; exit 2; }
docker info >/dev/null
docker compose version >/dev/null
git -C "$PWD" rev-parse --verify 'v1.0.0^{commit}' >/dev/null

action runner_ready "Docker-in-Docker engine $(docker version --format '{{.Server.Version}}'); requested observation ${DURATION_TEXT}"
CURRENT_STAGE=binary_build
action binaries_build_started "building tagged v1.0.0 agent and current agent/operator"
git -C "$PWD" archive v1.0.0 | tar -x -C "$WORK/v1"
(cd "$WORK/v1" && CGO_ENABLED=0 GOTOOLCHAIN=auto go build -trimpath -o "$BIN_DIR/captain-compose-mqtt-v1" ./cmd/captain-compose-mqtt)
CGO_ENABLED=0 GOTOOLCHAIN=auto go build -trimpath -o "$BIN_DIR/captain-compose-mqtt" ./cmd/captain-compose-mqtt
CGO_ENABLED=0 GOTOOLCHAIN=auto go build -trimpath -o "$BIN_DIR/captain-compose" ./cmd/captain-compose
V1_AGENT_SHA256=$(sha256sum "$BIN_DIR/captain-compose-mqtt-v1" | awk '{print $1}')
AGENT_SHA256=$(sha256sum "$BIN_DIR/captain-compose-mqtt" | awk '{print $1}')
OPERATOR_SHA256=$(sha256sum "$BIN_DIR/captain-compose" | awk '{print $1}')
check build passed "built tag v1.0.0 and current captain-compose-mqtt and captain-compose binaries"

CURRENT_STAGE=broker_tls_and_credentials
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$WORK/certs/ca.key" -out "$WORK/certs/ca.crt" -days 2 -subj '/CN=Captain Compose Pilot CA' -addext 'basicConstraints=critical,CA:TRUE' >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -keyout "$WORK/certs/server.key" -out "$WORK/certs/server.csr" -subj '/CN=127.0.0.1' >/dev/null 2>&1
printf 'subjectAltName=IP:127.0.0.1,DNS:localhost\nextendedKeyUsage=serverAuth\n' >"$WORK/certs/server.ext"
openssl x509 -req -in "$WORK/certs/server.csr" -CA "$WORK/certs/ca.crt" -CAkey "$WORK/certs/ca.key" -CAcreateserial -out "$WORK/certs/server.crt" -days 2 -sha256 -extfile "$WORK/certs/server.ext" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$WORK/certs/wrong-ca.key" -out "$WORK/certs/wrong-ca.crt" -days 2 -subj '/CN=Wrong Pilot CA' -addext 'basicConstraints=critical,CA:TRUE' >/dev/null 2>&1
printf '%s\n' "$PASSWORD" >"$CONFIG_DIR/password"
chmod 600 "$CONFIG_DIR/password"
docker run --rm -v "$WORK/config:/tmp/pilot" --entrypoint mosquitto_passwd "$MOSQUITTO_IMAGE" -b -c /tmp/pilot/passwords "$USERNAME" "$PASSWORD"
cp "$WORK/certs/ca.crt" "$CONFIG_DIR/ca.crt"
cp "$WORK/certs/server.crt" "$CONFIG_DIR/server.crt"
cp "$WORK/certs/server.key" "$CONFIG_DIR/server.key"
chmod 644 "$WORK/config/passwords" "$WORK/certs/server.key" "$WORK/certs/server.crt" "$WORK/certs/ca.crt"
cat >"$CONFIG_DIR/mosquitto.conf" <<EOF
persistence true
persistence_location /mosquitto/data/
per_listener_settings true
listener 1883 0.0.0.0
allow_anonymous false
password_file /mosquitto/config/passwords
listener 8883 0.0.0.0
allow_anonymous false
password_file /mosquitto/config/passwords
cafile /mosquitto/config/ca.crt
certfile /mosquitto/config/server.crt
keyfile /mosquitto/config/server.key
tls_version tlsv1.2
EOF
docker run --detach --name "$BROKER" --publish 127.0.0.1:18883:1883 --publish 127.0.0.1:18884:8883 \
  --mount "type=bind,source=$CONFIG_DIR,target=/mosquitto/config" "$MOSQUITTO_IMAGE" >/dev/null
if ! wait_for 'authenticated broker listener' 20 docker exec "$BROKER" mosquitto_pub -h 127.0.0.1 -p 1883 -u "$USERNAME" -P "$PASSWORD" -t "$COMMAND_TOPIC/health" -q 1 -m ready; then
  BROKER_LOG=$(docker logs --tail 40 "$BROKER" 2>&1 || true)
  echo "$BROKER_LOG" >&2
  fail broker_listener "private broker's authenticated plaintext listener failed its readiness probe"
fi
check broker passed "isolated authenticated MQTT broker has plain v1 and verified TLS listeners"
action broker_started "private test broker started with one-time user and generated CA"

CURRENT_STAGE=v1_accepted_request
operator_env_config "$CONFIG_DIR/password" "$WORK/certs/ca.crt" 45s
agent_config "$STATE_DIR" tcp://127.0.0.1:18883 true ''
"$BIN_DIR/captain-compose-mqtt-v1" --config "$CONFIG_DIR/agent.yaml" >"$WORK/v1-agent.log" 2>&1 &
V1_PID=$!
wait_for 'v1 agent subscription' 20 grep -q 'MQTT subscription ready' "$WORK/v1-agent.log" || fail v1_agent_start "tagged v1.0.0 agent did not subscribe: $(tail -n 20 "$WORK/v1-agent.log" 2>/dev/null | tr '\n' ' ' | cut -c1-500)"

LEGACY_REQUEST_ID=$(cat /proc/sys/kernel/random/uuid)
LEGACY_NAME=pilotdata
LEGACY_PAYLOAD=$(base64 -w0 tests/acceptance/fixtures/upgrade-v1.yaml)
jq -cn --arg id "$LEGACY_REQUEST_ID" --arg name "$LEGACY_NAME" --arg payload "$LEGACY_PAYLOAD" '{id:$id,type:"create",data:{name:$name,payload:$payload}}' >"$WORK/v1-request.json"
docker cp "$WORK/v1-request.json" "$BROKER:/tmp/v1-request.json" >/dev/null
docker exec "$BROKER" mosquitto_pub -h 127.0.0.1 -p 1883 -u "$USERNAME" -P "$PASSWORD" -t "$COMMAND_TOPIC" -q 1 -f /tmp/v1-request.json
wait_for 'v1 accepted journal request' 15 journal_recorded "$LEGACY_REQUEST_ID" pending || fail v1_accepted "v1.0.0 did not durably record the submitted request"
wait_for 'v1 workload started' 30 compose_waiting "$LEGACY_NAME" || fail v1_workload_start "v1.0.0 accepted request did not start its delayed test workload"
action v1_request_accepted "v1.0.0 journal contains request $LEGACY_REQUEST_ID before its delayed workload became healthy"
stop_agent
kill -TERM "$V1_PID" 2>/dev/null || true
wait "$V1_PID" 2>/dev/null || true
V1_PID=''
if wait_saved_event "$LEGACY_REQUEST_ID"; then fail v1_pending_window "legacy request completed before the v1 process was stopped"; fi
check v1_upgrade_seed passed "stopped v1.0.0 with its accepted request still pending in the real 1.0 journal format"
action v1_agent_stopped "v1 agent stopped with outstanding accepted request and deployment state preserved"

agent_config "$STATE_DIR" ssl://127.0.0.1:18884 false "$WORK/certs/ca.crt"
: >"$WORK/agent.log"
start_agent || fail upgraded_agent_start "current agent failed to resume the v1 journal state"
action current_agent_started "current agent reopened the v1.0.0 state directory and resumed its accepted request"
set +e
LEGACY_RESULT=$(operator wait "$LEGACY_REQUEST_ID" 2>&1)
LEGACY_RC=$?
set -e
if [[ $LEGACY_RC != 0 ]]; then
  fail v1_result "upgraded agent result query failed (exit $LEGACY_RC): $(printf '%s' "$LEGACY_RESULT" | tr '\n' ' ' | cut -c1-500); agent log: $(tail -n 12 "$WORK/agent.log" | tr '\n' ' ' | cut -c1-500)"
fi
json_result_success "$LEGACY_RESULT" || fail v1_result "v1 request result was not successful"
check v1_upgrade_and_outstanding_work passed "v1.0.0 accepted request completed after upgrade; result state $(jq -r '.state' <<<"$LEGACY_RESULT")"
REPEAT_RESULT=$(operator result "$LEGACY_REQUEST_ID") || fail duplicate_result_query "repeat result query failed"
json_result_success "$REPEAT_RESULT" || fail duplicate_result_query "repeat query did not return a successful saved event"
[[ $(jq -r '.result.id' <<<"$LEGACY_RESULT") == $(jq -r '.result.id' <<<"$REPEAT_RESULT") ]] || fail duplicate_result_query "repeated query returned a different saved event ID"
[[ $(jq -r '.result.request_id' <<<"$REPEAT_RESULT") == "$LEGACY_REQUEST_ID" ]] || fail duplicate_result_query "repeated query did not return the original request ID"
check duplicate_result_query passed "two independent v2 queries returned the same saved v1 lifecycle event"

DEPLOY_RESULT=$(operator deploy "$LEGACY_NAME" tests/acceptance/fixtures/upgrade-v1.yaml) || fail v2_legacy_adoption "v2 could not adopt legacy deployment state"
json_success "$DEPLOY_RESULT" || fail v2_legacy_adoption "v2 legacy-state adoption did not complete"
STATUS_RESULT=$(operator inspect "$LEGACY_NAME") || fail revision_output "could not read deployment revision after apply"
json_success "$STATUS_RESULT" || fail revision_output "inspect did not return deployment status after apply"
json_active_deployment "$STATUS_RESULT" || fail deployment_health "deployment did not become active with healthy services and no drift"
REVISION=$(jq -r '.result.desired_revision // empty' <<<"$STATUS_RESULT")
[[ "$REVISION" =~ ^[0-9a-f]{64}$ ]] || fail revision_output "deployment result omitted its SHA-256 desired revision"
check deployment_health passed "inspect observed active deployment with every service running and healthy and no drift"
check v2_legacy_adoption passed "v2 wrote revisioned deployment state while retaining the existing named volume"

set +e
STALE_RESULT=$(operator deploy "$LEGACY_NAME" tests/acceptance/fixtures/upgrade-v1.yaml --expected-revision "$(printf '0%.0s' {1..64})" 2>&1)
STALE_RC=$?
set -e
if [[ $STALE_RC != 4 ]] || ! jq -e '.code=="revision_conflict"' <<<"$STALE_RESULT" >/dev/null; then
  fail stale_revision "stale expected revision was not rejected before deployment"
fi
check stale_revision passed "wrong expected revision returned revision_conflict"

set +e
EXPIRED_RESULT=$(operator deploy expiryprobe tests/acceptance/fixtures/persistent-data.yaml --expires 1ns 2>&1)
EXPIRED_RC=$?
set -e
if [[ $EXPIRED_RC != 4 ]] || ! jq -e '.code=="expired"' <<<"$EXPIRED_RESULT" >/dev/null; then
  fail expired_command "already-expired v2 command was not rejected"
fi
! project_exists expiryprobe || fail expired_command "expired request unexpectedly created a Compose project"
check expired_command passed "1ns request expired before execution and created no workload"

# Queue two delayed operations durably, cut only the disposable broker, and let
# the independent execution worker finish both while the result outbox retries.
BACKLOG_A=$(operator deploy backloga tests/acceptance/fixtures/upgrade-v1.yaml --wait=false) || fail backlog_acceptance "broker did not accept backlog request A"
BACKLOG_B=$(operator deploy backlogb tests/acceptance/fixtures/upgrade-v1.yaml --wait=false) || fail backlog_acceptance "broker did not accept backlog request B"
ID_A=$(result_request_id "$BACKLOG_A")
ID_B=$(result_request_id "$BACKLOG_B")
[[ -n "$ID_A" && -n "$ID_B" ]] || fail backlog_acceptance "operator did not return request IDs"
wait_for 'backlog requests journaled' 10 journal_recorded "$ID_A" pending || fail backlog_acceptance "request A did not reach the durable inbox"
wait_for 'second backlog request journaled' 10 journal_recorded "$ID_B" pending || fail backlog_acceptance "request B did not reach the durable inbox"
wait_for 'backlog A workload started' 10 compose_waiting backloga || fail backlog_acceptance "request A did not start"
docker stop "$BROKER" >/dev/null
wait_for 'both accepted workloads complete' 45 bash -c 'docker ps --filter "label=com.docker.compose.project=cc-backloga" --filter health=healthy -q | grep -q . && docker ps --filter "label=com.docker.compose.project=cc-backlogb" --filter health=healthy -q | grep -q .' || fail backlog_independence "accepted operations did not both finish while MQTT was down"
wait_for 'outbox retry observed' 15 grep -q 'Event delivery pending' "$WORK/agent.log" || fail broker_outage "agent did not log pending event delivery during the outage"
action broker_stopped "stopped only $BROKER after both request IDs were present in the agent journal"
docker start "$BROKER" >/dev/null
wait_for 'broker restored' 20 docker exec "$BROKER" mosquitto_pub -h 127.0.0.1 -p 1883 -u "$USERNAME" -P "$PASSWORD" -t "$COMMAND_TOPIC/health" -q 1 -m restored
RESULT_A=$(operator result "$ID_A") || fail backlog_result_recovery "request A saved event was unavailable after reconnect"
RESULT_B=$(operator result "$ID_B") || fail backlog_result_recovery "request B saved event was unavailable after reconnect"
if ! json_result_success "$RESULT_A" || ! json_result_success "$RESULT_B"; then
  fail backlog_result_recovery "backlogged operations did not both return success"
fi
check broker_outage_backlog_independence passed "both accepted operations completed during broker outage; saved results were delivered after reconnect"

# Stop and reopen the agent against the same on-disk directory, then preserve and
# restore the named data volume plus state into a fresh process/state path.
stop_agent
action process_restart "stopped and restarted the agent process using the same state directory"
start_agent || fail process_restart "agent did not restart from its existing state directory"
STATUS_RESULT=$(operator inspect "$LEGACY_NAME") || fail process_restart "deployment status query failed after agent process restart"
json_success "$STATUS_RESULT" || fail process_restart "status query after restart failed"
check process_restart passed "current agent restarted with its persisted state and answered inspect"

VOLUME_PROJECT="cc-$LEGACY_NAME"
VOLUME=$(docker volume ls --filter "label=com.docker.compose.project=$VOLUME_PROJECT" --filter 'label=com.docker.compose.volume=pilot-data' --format '{{.Name}}' | head -n 1)
[[ -n "$VOLUME" ]] || fail data_backup "expected named data volume was not present"
VOLUME_LABELS=$(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}|{{index .Labels "com.docker.compose.volume"}}' "$VOLUME")
[[ "$VOLUME_LABELS" == "$VOLUME_PROJECT|pilot-data" ]] || fail data_backup "volume label ownership did not match this pilot deployment"
docker run --rm --volume "$VOLUME:/pilot-data:ro" --volume "$WORK:/backup" alpine:3.22 sh -c 'tar -cf /backup/data.tar -C /pilot-data .'
stop_agent
cp -a "$STATE_DIR" "$WORK/state-backup"
docker compose --env-file /dev/null --project-name "cc-$LEGACY_NAME" --file tests/acceptance/fixtures/upgrade-v1.yaml down --remove-orphans >/dev/null
VOLUME_LABELS=$(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}|{{index .Labels "com.docker.compose.volume"}}' "$VOLUME")
[[ "$VOLUME_LABELS" == "$VOLUME_PROJECT|pilot-data" ]] || fail state_restore "refusing to remove a volume no longer labeled for this pilot deployment"
docker volume rm "$VOLUME" >/dev/null
docker volume create "$VOLUME" >/dev/null
docker run --rm --volume "$VOLUME:/pilot-data" --volume "$WORK:/backup" alpine:3.22 sh -c 'tar -xf /backup/data.tar -C /pilot-data'
mv "$STATE_DIR" "$WORK/state-before-restore"
cp -a "$WORK/state-backup" "$STATE_DIR"
start_agent || fail state_restore "fresh agent did not open the restored state copy"
[[ $(docker run --rm --volume "$VOLUME:/pilot-data:ro" alpine:3.22 cat /pilot-data/marker) == v1-data ]] || fail state_restore "restored named volume did not contain the original marker"
RESTORED_RESULT=$(operator deploy "$LEGACY_NAME" tests/acceptance/fixtures/upgrade-v1.yaml --expected-revision "$REVISION") || fail state_restore "restored desired state could not recreate its workload"
json_success "$RESTORED_RESULT" || fail state_restore "restored workload apply failed"
RESTORED_CID=$(docker ps --filter "label=com.docker.compose.project=cc-$LEGACY_NAME" --filter status=running --format '{{.ID}}' | head -n 1)
[[ -n "$RESTORED_CID" && $(docker exec "$RESTORED_CID" cat /pilot-data/marker) == v1-data ]] || fail state_restore "recreated workload did not read preserved data"
check state_and_data_restore passed "copied state and restored volume were opened by a fresh agent process; workload data marker remained intact"

# Force a daemon preflight against an unreachable endpoint without changing the
# live agent's Docker environment or its state lock.
sed "s|^state_dir: .*|state_dir: $WORK/no-docker-state|" "$CONFIG_DIR/agent.yaml" >"$CONFIG_DIR/no-docker.yaml"
set +e
NO_DOCKER_OUTPUT=$(DOCKER_HOST=tcp://127.0.0.1:1 "$BIN_DIR/captain-compose-mqtt" --config "$CONFIG_DIR/no-docker.yaml" --check 2>&1)
NO_DOCKER_RC=$?
set -e
[[ $NO_DOCKER_RC != 0 && "$NO_DOCKER_OUTPUT" == *'Docker readiness'* ]] || fail docker_unavailable "agent preflight did not clearly reject its isolated unreachable Docker endpoint"
check docker_unavailable passed "agent --check failed with Docker readiness against 127.0.0.1:1"

DISK_MOUNT="$WORK/diskfull"
mkdir -p "$DISK_MOUNT"
mount -t tmpfs -o size=1m,mode=0700 tmpfs "$DISK_MOUNT" || fail disk_full "could not mount the isolated 1 MiB tmpfs"
dd if=/dev/zero of="$DISK_MOUNT/fill" bs=1024 count=1024 status=none 2>/dev/null || true
DISK_COMMAND_TOPIC="$COMMAND_TOPIC/diskfull"
DISK_EVENT_TOPIC="$EVENT_TOPIC/diskfull"
sed -e "s|^state_dir: .*|state_dir: $DISK_MOUNT/state|" \
  -e "s|^  client_id: .*|  client_id: $PILOT_ID-diskfull|" \
  -e "s|^listener_topic: .*|listener_topic: $DISK_COMMAND_TOPIC|" \
  -e "s|^publisher_topic: .*|publisher_topic: $DISK_EVENT_TOPIC|" \
  "$CONFIG_DIR/agent.yaml" >"$CONFIG_DIR/diskfull-agent.yaml"
cat >"$CONFIG_DIR/diskfull-operator.yaml" <<EOF
environments:
  pilot:
    broker_url: ssl://127.0.0.1:18884
    command_topic: $DISK_COMMAND_TOPIC
    event_topic: $DISK_EVENT_TOPIC
    username: $USERNAME
    password_file: $CONFIG_DIR/password
    tls:
      ca_cert_path: $WORK/certs/ca.crt
    timeout: 20s
    retry_interval: 1s
EOF
"$BIN_DIR/captain-compose-mqtt" --config "$CONFIG_DIR/diskfull-agent.yaml" >"$WORK/diskfull-agent.log" 2>&1 &
DISK_AGENT_PID=$!
if wait_for 'disk-full agent subscription' 12 grep -q 'MQTT subscription ready' "$WORK/diskfull-agent.log"; then
  DISK_REQUEST=$("$BIN_DIR/captain-compose" --config "$CONFIG_DIR/diskfull-operator.yaml" --environment pilot --json deploy diskfull tests/acceptance/fixtures/persistent-data.yaml --wait=false) || fail disk_full "broker rejected the disk-full fixture command"
  DISK_REQUEST_ID=$(result_request_id "$DISK_REQUEST")
  [[ -n "$DISK_REQUEST_ID" ]] || fail disk_full "operator omitted disk-full request ID"
  # shellcheck disable=SC2317,SC2329 # Invoked by name through wait_for's command dispatch.
  disk_agent_exited() { ! kill -0 "$DISK_AGENT_PID" 2>/dev/null; }
  wait_for 'disk-full agent exits after journal failure' 15 disk_agent_exited || fail disk_full "full journal did not stop the agent within the bounded window"
  wait "$DISK_AGENT_PID" 2>/dev/null || true
else
  for _ in $(seq 1 20); do
    kill -0 "$DISK_AGENT_PID" 2>/dev/null || break
    sleep 0.1
  done
  kill -TERM "$DISK_AGENT_PID" 2>/dev/null || true
  wait "$DISK_AGENT_PID" 2>/dev/null || true
fi
DISK_AGENT_PID=''
if grep -Eqi 'no space left|ENOSPC|disk quota' "$WORK/diskfull-agent.log"; then
  ! project_exists diskfull || fail disk_full "disk-full request unexpectedly created a workload"
  check disk_full passed "agent reported bounded state/journal failure on a capped 1 MiB tmpfs; no deployment was created"
else
  fail disk_full "capped tmpfs did not produce a reported no-space persistence failure"
fi
umount "$DISK_MOUNT"
DISK_MOUNT=''

# Reject bad MQTT credentials and an untrusted server certificate through the
# real operator client without sending deployment commands.
cp "$CONFIG_DIR/password" "$CONFIG_DIR/wrong-password"
printf 'invalid pilot password\n' >"$CONFIG_DIR/wrong-password"
operator_env_config "$CONFIG_DIR/wrong-password" "$WORK/certs/ca.crt" 15s
set +e
BAD_CREDENTIALS=$(operator doctor 2>&1)
BAD_CREDENTIALS_RC=$?
set -e
[[ $BAD_CREDENTIALS_RC != 0 && "$BAD_CREDENTIALS" == *'cannot connect to MQTT broker'* ]] || fail credential_failure "operator did not reject the test broker's invalid credentials"
check credential_failure passed "operator refused an authenticated TLS broker session with the wrong password"
operator_env_config "$CONFIG_DIR/password" "$WORK/certs/wrong-ca.crt" 15s
set +e
BAD_CERT=$(operator doctor 2>&1)
BAD_CERT_RC=$?
set -e
[[ $BAD_CERT_RC != 0 && "$BAD_CERT" == *'cannot connect to MQTT broker'* ]] || fail certificate_failure "operator did not reject the untrusted broker certificate"
check certificate_failure passed "operator refused TLS with an unrelated CA"
operator_env_config "$CONFIG_DIR/password" "$WORK/certs/ca.crt" 45s

OBSERVATION_START_UTC=$(date -u +%Y-%m-%dT%H:%M:%SZ)
OBSERVATION_START_SECONDS=$(date -u +%s)
action observation_started "all deterministic acceptance actions passed; periodic inspection continues for $DURATION_TEXT"
while :; do
  sample_start=$(date -u +%s)
  if (( LAST_SAMPLE_SECONDS > 0 )); then
    sample_gap=$((sample_start - LAST_SAMPLE_SECONDS))
    (( sample_gap > MAX_SAMPLE_GAP_SECONDS )) && MAX_SAMPLE_GAP_SECONDS=$sample_gap
    (( sample_gap <= MAX_SAMPLE_GAP_LIMIT )) || fail observation_gap "no successful observation for ${sample_gap}s; limit is ${MAX_SAMPLE_GAP_LIMIT}s, so suspended time is not counted as continuous observation"
  fi
  kill -0 "$AGENT_PID" 2>/dev/null || fail observation_agent_alive "agent exited during the observation window"
  docker inspect -f '{{.State.Running}}' "$BROKER" 2>/dev/null | grep -qx true || fail observation_broker_alive "private broker exited during the observation window"
  SAMPLE=$(operator inspect "$LEGACY_NAME") || fail observation_query "deployment inspection failed during the observation window"
  json_active_deployment "$SAMPLE" || fail observation_query "deployment was not active, healthy, and drift-free during the observation window"
  sample_end=$(date -u +%s)
  if (( LAST_SAMPLE_SECONDS > 0 )); then
    sample_gap=$((sample_end - LAST_SAMPLE_SECONDS))
    (( sample_gap > MAX_SAMPLE_GAP_SECONDS )) && MAX_SAMPLE_GAP_SECONDS=$sample_gap
    (( sample_gap <= MAX_SAMPLE_GAP_LIMIT )) || fail observation_gap "no successful observation for ${sample_gap}s; limit is ${MAX_SAMPLE_GAP_LIMIT}s, so suspended time is not counted as continuous observation"
  fi
  MONITOR_SAMPLES=$((MONITOR_SAMPLES + 1))
  LAST_SAMPLE_SECONDS=$sample_end
  write_report
  observation_elapsed=$(( $(date -u +%s) - OBSERVATION_START_SECONDS ))
  if (( observation_elapsed >= DURATION_SECONDS && MONITOR_SAMPLES >= 2 )); then break; fi
  remaining=$((DURATION_SECONDS - observation_elapsed))
  (( remaining > 0 )) || remaining=1
  interval=$(( DURATION_SECONDS / 4 ))
  (( interval < 1 )) && interval=1
  (( interval > 30 )) && interval=30
  sleep "$(( remaining < interval ? remaining : interval ))"
done
check duration_observed passed "observed for $(( $(date -u +%s) - OBSERVATION_START_SECONDS ))s; completed $MONITOR_SAMPLES periodic deployment inspections"
action observation_finished "agent and broker remained responsive through the requested observation duration"
exit 0
