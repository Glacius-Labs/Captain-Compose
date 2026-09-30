#!/usr/bin/env bash
# Verify and install both release executables without changing host services.
set -euo pipefail
version=''
prefix="$HOME/.local/bin"
archive_dir=''
attestation_dir=''
checksum_only=false
usage() {
  cat <<'USAGE'
Usage: install.sh --version X.Y.Z [--prefix DIR] [--archive-dir DIR]
                  [--attestation-dir DIR | --checksum-only]

By default, verifies GitHub build provenance for the exact release tag and
release workflow, then verifies checksums. --checksum-only skips provenance
and is intended only for controlled bootstrap situations.
USAGE
}
while (($#)); do
  case "$1" in
    --version) version=${2:?Missing version}; shift 2 ;;
    --prefix) prefix=${2:?Missing prefix}; shift 2 ;;
    --archive-dir) archive_dir=${2:?Missing archive directory}; shift 2 ;;
    --attestation-dir) attestation_dir=${2:?Missing attestation directory}; shift 2 ;;
    --checksum-only) checksum_only=true; shift ;;
    --help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || { echo 'An explicit semantic version is required' >&2; exit 2; }
if "$checksum_only" && [[ -n "$attestation_dir" ]]; then
  echo '--checksum-only cannot be combined with --attestation-dir' >&2
  exit 2
fi
case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) echo 'Use the Windows release ZIP on Windows' >&2; exit 2 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo 'Unsupported architecture' >&2; exit 2 ;; esac
archive="captain-compose_${version}_${os}_${arch}.tar.gz"
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
if [[ -n "$archive_dir" ]]; then
  cp -- "$archive_dir/$archive" "$work/$archive"
  cp -- "$archive_dir/checksums.txt" "$work/checksums.txt"
else
  base="https://github.com/Glacius-Labs/Captain-Compose/releases/download/v$version"
  curl --fail --show-error --silent --location --proto '=https' --tlsv1.2 "$base/$archive" -o "$work/$archive"
  curl --fail --show-error --silent --location --proto '=https' --tlsv1.2 "$base/checksums.txt" -o "$work/checksums.txt"
fi
matches=$(awk -v name="$archive" '$2 == name || $2 == "*" name { print $1 }' "$work/checksums.txt")
[[ "$matches" =~ ^[a-f0-9]{64}$ ]] || { echo 'Missing, duplicate or invalid release checksum' >&2; exit 1; }
if command -v sha256sum >/dev/null; then actual=$(sha256sum "$work/$archive"); else actual=$(shasum -a 256 "$work/$archive"); fi
[[ "${actual%% *}" == "$matches" ]] || { echo 'Release checksum mismatch' >&2; exit 1; }

if ! "$checksum_only"; then
  command -v gh >/dev/null || { echo 'GitHub CLI (gh) is required for provenance verification; use --checksum-only only for controlled bootstrap' >&2; exit 1; }
  verify_args=("$work/$archive" --repo Glacius-Labs/Captain-Compose
    --signer-workflow Glacius-Labs/Captain-Compose/.github/workflows/release.yml
    --source-ref "refs/tags/v$version" --deny-self-hosted-runners)
  if [[ -n "$attestation_dir" ]]; then
    bundle="$attestation_dir/$archive.jsonl"
    trusted_root="$attestation_dir/trusted_root.jsonl"
    [[ -s "$bundle" ]] || { echo "Missing offline attestation bundle: $bundle" >&2; exit 1; }
    [[ -s "$trusted_root" ]] || { echo "Missing offline trusted root: $trusted_root" >&2; exit 1; }
    verify_args+=(--bundle "$bundle" --custom-trusted-root "$trusted_root")
  fi
  gh attestation verify "${verify_args[@]}"
fi

# Extract only the named regular files. Never trust archive paths to choose install targets.
tar -xOf "$work/$archive" ./captain-compose-mqtt > "$work/captain-compose-mqtt"
tar -xOf "$work/$archive" ./captain-compose > "$work/captain-compose"
chmod 0755 "$work/captain-compose-mqtt" "$work/captain-compose"
mqtt_version=$("$work/captain-compose-mqtt" --version)
cli_version=$("$work/captain-compose" --version)
case " $mqtt_version " in *" $version "*) ;; *) echo "MQTT binary version does not match requested release $version" >&2; exit 1 ;; esac
case " $cli_version " in *" $version "*) ;; *) echo "CLI binary version does not match requested release $version" >&2; exit 1 ;; esac
printf '%s\n%s\n' "$mqtt_version" "$cli_version"
mkdir -p -- "$prefix"
for binary in captain-compose-mqtt captain-compose; do
  install -m 0755 "$work/$binary" "$prefix/.$binary.new"
  mv -f -- "$prefix/.$binary.new" "$prefix/$binary"
done
echo "Installed both Captain Compose commands in $prefix; ensure it is on PATH."
