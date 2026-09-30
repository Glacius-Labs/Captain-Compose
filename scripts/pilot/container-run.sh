#!/bin/sh
set -eu

/usr/local/bin/dockerd-entrypoint.sh dockerd >/tmp/captain-compose-pilot-dockerd.log 2>&1 &
dockerd_pid=$!
stop_dockerd() {
  kill "$dockerd_pid" 2>/dev/null || true
  wait "$dockerd_pid" 2>/dev/null || true
}
trap stop_dockerd EXIT HUP INT TERM

# The desktop app may inject a tcp://docker endpoint for its own runner. Pin every
# pilot docker client and the agent's docker subprocesses to this private socket.
unset DOCKER_CONTEXT DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION
DOCKER_HOST=unix:///var/run/docker.sock
export DOCKER_HOST

ready=0
for _ in $(seq 1 90); do
  if docker info >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
if [ "$ready" != 1 ]; then
  cat /tmp/captain-compose-pilot-dockerd.log >&2
  echo 'The private nested Docker daemon did not become ready.' >&2
  exit 1
fi

apk add --no-cache bash jq openssl go git docker-cli-compose coreutils shellcheck
git config --global --add safe.directory "$PWD"
shellcheck scripts/pilot/*.sh
bash scripts/pilot/run.sh "$@"
