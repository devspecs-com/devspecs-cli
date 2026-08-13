#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
child_dir=$(pwd -P)
source_root=$repo_root
if command -v cygpath >/dev/null 2>&1; then
  source_root=$(cygpath -w "$repo_root")
  child_dir=$(cygpath -w "$child_dir")
fi

exec go -C "$repo_root" run ./scripts/dev-env --source-root "$source_root" --child-dir "$child_dir" "$@"
