#!/usr/bin/env bash
set -euo pipefail

if (( $# != 1 )); then
  echo "usage: $0 OUTPUT_DIRECTORY" >&2
  exit 2
fi

output=$1
mkdir -p "$output"
manifest="$output/modules.tsv"
printf 'module\tversion\tlicense_files\n' > "$manifest"

template='{{with .Module}}{{if and (not .Main) .Dir}}{{.Path}}{{"\t"}}{{.Version}}{{"\t"}}{{.Dir}}{{end}}{{end}}'

while IFS=$'\t' read -r module version directory; do
  [[ -n "$module" && -n "$directory" ]] || continue
  safe_name=$(printf '%s@%s' "$module" "$version" | tr '/:' '__')
  destination="$output/$safe_name"
  mkdir -p "$destination"

  shopt -s nullglob
  candidates=("$directory"/LICENSE* "$directory"/COPYING* "$directory"/NOTICE*)
  shopt -u nullglob
  if (( ${#candidates[@]} == 0 )); then
    echo "third-party-licenses: no license file found for $module $version" >&2
    exit 1
  fi

  copied=()
  for source in "${candidates[@]}"; do
    [[ -f "$source" ]] || continue
    name=$(basename "$source")
    cp "$source" "$destination/$name"
    copied+=("$name")
  done
  if (( ${#copied[@]} == 0 )); then
    echo "third-party-licenses: no regular license file found for $module $version" >&2
    exit 1
  fi
  joined=$(IFS=,; echo "${copied[*]}")
  printf '%s\t%s\t%s\n' "$module" "$version" "$joined" >> "$manifest"
done < <(go list -deps -f "$template" ./cmd/nas-governance | LC_ALL=C sort -u)

count=$(($(wc -l < "$manifest") - 1))
if (( count == 0 )); then
  echo "third-party-licenses: no runtime modules discovered" >&2
  exit 1
fi

echo "third-party-licenses: collected licenses for $count runtime modules"
