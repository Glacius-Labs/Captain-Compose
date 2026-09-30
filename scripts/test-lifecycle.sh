#!/usr/bin/env bash
set -euo pipefail
[[ "$EUID" == 0 ]] || { echo 'Run this test with sudo in a disposable environment' >&2; exit 1; }
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
root=$(mktemp -d /tmp/captain-compose-lifecycle-test.XXXXXX)
trap 'rm -rf -- "$root"' EXIT
mkdir -p "$root/var/tmp" "$root/var/backups" "$root/var/lib/captain-compose/journal" \
  "$root/etc/captain-compose/docker" "$root/fakebin"
printf 'journal-state\n' > "$root/var/lib/captain-compose/journal/state.json"
printf 'mqtt-secret\n' > "$root/etc/captain-compose/mqtt-password"
printf 'state_dir: /var/lib/captain-compose\n' > "$root/etc/captain-compose/config.yaml"
printf '{"auths":{"registry.example":{"auth":"secret"}}}\n' > "$root/etc/captain-compose/docker/config.json"
cat > "$root/fakebin/systemctl" <<'SYSTEMCTL'
#!/usr/bin/env bash
case "$1" in
  is-active) [[ "${CAPTAIN_COMPOSE_TEST_SERVICE_ACTIVE:-0}" == 1 ]] ;;
  stop) exit 0 ;;
  *) exit 0 ;;
esac
SYSTEMCTL
chmod 0755 "$root/fakebin/systemctl"
export CAPTAIN_COMPOSE_ROOT="$root"
export PATH="$root/fakebin:$PATH"
archive="$root/var/backups/agent.tar.gz"
bash "$repo/scripts/backup.sh" "$archive"
test "$(stat -c '%a' "$archive")" = 600

# Restore must reject existing destinations before making any change.
printf 'keep-current\n' > "$root/var/lib/captain-compose/journal/state.json"
if bash "$repo/scripts/restore.sh" --archive "$archive" > "$root/refuse.log" 2>&1; then
  echo 'Restore unexpectedly overwrote existing directories' >&2
  exit 1
fi
grep -q 'Restore destination already exists' "$root/refuse.log"
grep -q 'keep-current' "$root/var/lib/captain-compose/journal/state.json"

# Reject symlinked parents before staging or moving data outside the chosen root.
unsafe_parent="$root/unsafe-parent"
mkdir -p "$unsafe_parent/outside/lib" "$unsafe_parent/etc" "$unsafe_parent/var/tmp"
ln -s "$unsafe_parent/outside" "$unsafe_parent/var/lib"
CAPTAIN_COMPOSE_ROOT="$unsafe_parent" bash "$repo/scripts/restore.sh" --archive "$archive" > "$unsafe_parent/parent.log" 2>&1 && {
  echo 'Restore unexpectedly accepted a symlinked parent' >&2
  exit 1
}
grep -q 'Restore parent must be an existing real directory' "$unsafe_parent/parent.log"
test ! -e "$unsafe_parent/outside/lib/captain-compose"

# Restore into a fresh staging root and verify state plus credentials/configuration.
fresh="$root/fresh"
mkdir -p "$fresh/var/tmp" "$fresh/var/backups" "$fresh/var/lib" "$fresh/etc"
CAPTAIN_COMPOSE_ROOT="$fresh" bash "$repo/scripts/restore.sh" --archive "$archive"
grep -q journal-state "$fresh/var/lib/captain-compose/journal/state.json"
grep -q mqtt-secret "$fresh/etc/captain-compose/mqtt-password"
grep -q registry.example "$fresh/etc/captain-compose/docker/config.json"

# Explicit replacement preserves old trees as siblings and installs the staged copy.
replace="$root/replace"
mkdir -p "$replace/var/tmp" "$replace/var/lib/captain-compose" "$replace/etc/captain-compose"
printf 'old-state\n' > "$replace/var/lib/captain-compose/old"
printf 'old-config\n' > "$replace/etc/captain-compose/old"
CAPTAIN_COMPOSE_ROOT="$replace" bash "$repo/scripts/restore.sh" --archive "$archive" \
  --replace-existing --confirm-replace state-and-config
grep -q journal-state "$replace/var/lib/captain-compose/journal/state.json"
grep -q mqtt-secret "$replace/etc/captain-compose/mqtt-password"
grep -q old-state "$replace"/var/lib/captain-compose.pre-restore-*/old
grep -q old-config "$replace"/etc/captain-compose.pre-restore-*/old

# Inject failure between the two directory installs and verify rollback restores both.
failure="$root/failure"
mkdir -p "$failure/var/tmp" "$failure/var/lib/captain-compose" "$failure/etc/captain-compose"
printf 'before-state\n' > "$failure/var/lib/captain-compose/old"
printf 'before-config\n' > "$failure/etc/captain-compose/old"
if CAPTAIN_COMPOSE_ROOT="$failure" CAPTAIN_COMPOSE_TEST_FAIL_AFTER_STATE_INSTALL=1 \
  bash "$repo/scripts/restore.sh" --archive "$archive" --replace-existing \
    --confirm-replace state-and-config > "$failure/failure.log" 2>&1; then
  echo 'Injected restore failure unexpectedly succeeded' >&2
  exit 1
fi
grep -q before-state "$failure/var/lib/captain-compose/old"
grep -q before-config "$failure/etc/captain-compose/old"

# Reject a symlink entry before extraction.
python3 - "$root/unsafe.tar.gz" <<'PY'
import io, sys, tarfile
with tarfile.open(sys.argv[1], "w:gz") as archive:
    for name in ("var/lib/captain-compose", "etc/captain-compose", "etc/captain-compose/config.yaml"):
        item = tarfile.TarInfo(name)
        if name.endswith("config.yaml"):
            payload = b"state_dir: ./state\n"
            item.size = len(payload)
            archive.addfile(item, io.BytesIO(payload))
        else:
            item.type = tarfile.DIRTYPE
            archive.addfile(item)
    item = tarfile.TarInfo("etc/captain-compose/escape")
    item.type = tarfile.SYMTYPE
    item.linkname = "/etc/passwd"
    archive.addfile(item)
PY
if python3 "$repo/scripts/archive-safety.py" validate "$root/unsafe.tar.gz" > "$root/unsafe.log" 2>&1; then
  echo 'Unsafe archive symlink was accepted' >&2
  exit 1
fi
grep -q 'unsupported link or special file' "$root/unsafe.log"
echo 'Backup, restore, explicit replacement, rollback and archive safety checks passed.'
