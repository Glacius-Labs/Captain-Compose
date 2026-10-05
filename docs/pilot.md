# Disposable production acceptance pilot

`scripts/pilot/run.sh` drives the real 1.0 and current binaries through an isolated
Linux environment. It creates a privileged nested Docker daemon, a private MQTT
broker, unique test containers, and test-only credentials inside that daemon. It does
not mount the Windows/Linux host Docker socket into the agent and does not operate on
host Docker workloads. The outer disposable runner is removed after the run.

The environment needs Docker Desktop with the Linux engine on Windows, or Docker
Engine on Linux, plus permission to start a privileged container. The runner uses
pinned Docker and Mosquitto images, installs the Go compiler in the disposable outer
container, and downloads Go modules; plan for network access and image storage. It
creates no cloud resources. A live JSON report captures UTC start and end times,
setup and observation elapsed times, checks, observed actions, failures, and cases
that this container cannot verify.

The runner generates a private broker CA and server certificate with validity beyond
the requested observation duration plus a one-day buffer. It checks that lifetime
before starting the broker; an expired test certificate would invalidate a long run.

Run the short CI pilot on Linux:

```bash
docker run --rm --privileged \
  --cpus 2 --memory 4g \
  --mount type=bind,source="$PWD",target=/workspace \
  --workdir /workspace \
  --env CAPTAIN_PILOT_DIND=1 \
  docker:29.4.1-dind \
  sh scripts/pilot/container-run.sh --duration 2m
```

On Windows PowerShell, use the same Linux container runner so Go and shell path
handling happen inside Linux:

```powershell
$repo = (Get-Location).Path
docker run --rm --privileged `
  --cpus 2 --memory 4g `
  --mount "type=bind,source=$repo,target=/workspace" `
  --workdir /workspace `
  --env CAPTAIN_PILOT_DIND=1 `
  docker:29.4.1-dind `
  sh scripts/pilot/container-run.sh --duration 2m
```

Use `--duration 72h` for a three-day observation. The runner keeps its isolated broker
and agent alive for that duration, periodically queries the recorded deployment, and
then writes the report and removes only resources named by that invocation. Set
`--report /workspace/path/to/report.json` to choose a report path; by default the
report is written under `scripts/pilot/` on the mounted workspace and refreshed after
each check and periodic inspection. The default report filename is ignored by Git.
For a multi-day run, the outer container may run detached. The Docker daemon and host
must remain awake and available; this command does not create a scheduler, cloud VM,
or remote notification service. The runner fails if it misses a successful inspection
for more than 120 seconds, so suspended time cannot silently count as observed uptime.
The live report includes the maximum sample gap, source revision and dirty flag,
architecture, tool versions, and SHA-256 hashes for all three built binaries. Keep the
source read-only and put the report in a separate bind-mounted directory, for example:

```bash
repo="$PWD"
reports="$(dirname "$repo")/captain-compose-pilot-reports"
mkdir -p "$reports"
docker run --rm --detach --name captain-compose-pilot-72h --restart=no \
  --privileged --cpus 2 --memory 4g \
  --log-driver local --log-opt max-size=10m --log-opt max-file=3 \
  --mount "type=bind,source=$repo,target=/workspace,readonly" \
  --mount "type=bind,source=$reports,target=/reports" \
  --workdir /workspace --env CAPTAIN_PILOT_DIND=1 \
  docker:29.4.1-dind \
  sh scripts/pilot/container-run.sh --duration 72h --report /reports/pilot-72h.json
```

Inspect progress with `docker logs captain-compose-pilot-72h` and the live JSON at
`$reports/pilot-72h.json`. The container removes itself on completion; the report
remains in the separate host directory. Stop only that named container to cancel the
run. A forced stop is not a simulated sudden-power-loss test.

The pilot exercises a v1.0.0 pending journal request, upgrades that state to the
current agent, checks duplicate result queries, broker disconnection and saved-result
recovery, expiry and revision rejection, a same-state agent restart, a simulated
state/data restore, disk-full on capped tmpfs, missing Docker access, and MQTT
credential/TLS failures. CI runs the two-minute pilot on amd64 and native ARM64 Linux
runners. Separate Linux CI checks install and start the systemd service. Read [the
acceptance matrix](acceptance-matrix.md) for exact boundaries; these CI checks still
do not establish physical reboot or sudden power-loss behavior.
