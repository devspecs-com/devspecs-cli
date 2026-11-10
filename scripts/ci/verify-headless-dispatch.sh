#!/bin/sh

set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo 'usage: verify-headless-dispatch.sh <linux-archive.tar.gz> <checksums.txt> [orchestration-test-binary]' >&2
  exit 2
fi
if [ "$(uname -s)" != Linux ]; then
  echo 'run the archive proof in Linux' >&2
  exit 2
fi

archive=$1
checksums=$2
root=$(cd "$(dirname "$0")/../.." && pwd -P)
name=${archive##*/}
expected=$(awk -v name="$name" 'NF == 2 && ($2 == name || $2 == "*" name) { print $1 }' "$checksums")
case "$expected" in
  ''|*[!0-9a-f]*) echo 'archive has no unique SHA-256 entry' >&2; exit 1 ;;
esac
if [ "${#expected}" -ne 64 ]; then
  echo 'archive checksum must be SHA-256' >&2
  exit 1
fi
actual=$(sha256sum -- "$archive")
if [ "${actual%% *}" != "$expected" ]; then
  echo 'archive SHA-256 mismatch' >&2
  exit 1
fi

entries=$(tar -tzf "$archive")
if [ "$(printf '%s\n' "$entries" | grep -cx ds)" -ne 1 ] ||
   printf '%s\n' "$entries" | grep -Eq '(^|/)ds-orchestrate(\.exe)?$'; then
  echo 'expected one ds binary and no orchestration companion' >&2
  exit 1
fi

temp_root=$(cd "${TMPDIR:-/tmp}" && pwd -P)
work=$(mktemp -d "$temp_root/devspecs-headless.XXXXXX")
case "$work" in
  "$temp_root"/devspecs-headless.*) ;;
  *) echo 'unexpected installation directory' >&2; exit 1 ;;
esac
trap 'rm -rf -- "$work"' EXIT
mkdir "$work/bin" "$work/home"
tar -xOzf "$archive" ds > "$work/bin/ds"
test -s "$work/bin/ds"
chmod 700 "$work/bin/ds"

export DEVSPECS_HOME="$work/home"
export DEVSPECS_TELEMETRY=0
export DEVSPECS_CONFORMANCE_DS="$work/bin/ds"
export DEVSPECS_CONFORMANCE_SPECS="$root/specs"
export PATH="$work/bin:$PATH"
ds --version

pattern='^Test(HeadlessHTTPDispatchConformance|HTTPDispatch|BuildHTTPDispatchRequest|AdmitHTTPDispatchReceipt|WriteHTTPDispatch(Receipt|Pending|Handle)|ReadHTTPDispatch(Pending|Handle|StoredReceipt)|LoopbackDispatch)'
if [ "$#" -eq 3 ]; then
  tests=$("$3" -test.list '^TestHeadlessHTTPDispatchConformance$')
  printf '%s\n' "$tests" | grep -qx TestHeadlessHTTPDispatchConformance
  "$3" -test.run "$pattern" -test.count=1 -test.timeout=150s -test.v
else
  cd "$root"
  go test -p 1 ./internal/orchestration -run "$pattern" -count=1 -timeout=150s -v
fi
