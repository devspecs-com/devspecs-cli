#!/usr/bin/env bash

set -euo pipefail

dist_dir="${1:-dist}"
archive_count=0

while IFS= read -r archive; do
  archive_count=$((archive_count + 1))
  if [[ "$archive" == *.zip ]]; then
    entries=$(unzip -Z1 "$archive")
  else
    entries=$(tar -tzf "$archive")
  fi
  grep -Eq '(^|/)ds(\.exe)?$' <<<"$entries"
  grep -Eq '(^|/)ds-orchestrate(\.exe)?$' <<<"$entries"
done < <(find "$dist_dir" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) -print)

test "$archive_count" -eq 5

deb_count=0
while IFS= read -r package; do
  deb_count=$((deb_count + 1))
  entries=$(dpkg-deb --contents "$package")
  grep -Eq '/usr/bin/ds$' <<<"$entries"
  grep -Eq '/usr/bin/ds-orchestrate$' <<<"$entries"
done < <(find "$dist_dir" -maxdepth 1 -type f -name '*.deb' -print)

test "$deb_count" -eq 2

package_count=$(jq '[.[] | select(.type == "Linux Package")] | length' "$dist_dir/artifacts.json")
complete_package_count=$(jq '[
  .[]
  | select(.type == "Linux Package")
  | select(
      ([.extra.Files[].dst] | index("/usr/bin/ds")) != null
      and ([.extra.Files[].dst] | index("/usr/bin/ds-orchestrate")) != null
    )
] | length' "$dist_dir/artifacts.json")
test "$package_count" -eq 4
test "$complete_package_count" -eq 4

formula="$dist_dir/homebrew/Formula/devspecs.rb"
ds_install_count=$(grep -Fc 'bin.install "ds"' "$formula")
orchestration_install_count=$(grep -Fc 'bin.install "ds-orchestrate"' "$formula")
test "$ds_install_count" -gt 0
test "$orchestration_install_count" -eq "$ds_install_count"
grep -Fq 'system "#{bin}/ds", "--version"' "$formula"
grep -Fq 'system "#{bin}/ds-orchestrate", "--version"' "$formula"

scoop="$dist_dir/scoop/devspecs.json"
jq -e '.architecture["64bit"].bin | index("ds.exe") != null' "$scoop" >/dev/null
jq -e '.architecture["64bit"].bin | index("ds-orchestrate.exe") != null' "$scoop" >/dev/null

printf 'release package smoke passed: %d archives, %d Linux packages, Homebrew, and Scoop\n' \
  "$archive_count" "$package_count"
