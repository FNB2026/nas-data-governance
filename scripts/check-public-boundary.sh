#!/usr/bin/env bash
set -euo pipefail

failed=0

if ! grep -qx 'module github.com/FNB2026/nas-data-governance' go.mod; then
  echo "public-boundary: go.mod must use the canonical public module path" >&2
  failed=1
fi

while IFS= read -r -d '' path; do
  case "$path" in
    .DS_Store|*/.DS_Store|*.db|*.db-shm|*.db-wal|*.sqlite|*.sqlite3|*.jsonl|*.log|*.pem|*.key|*.docx|*.xmind|*.pages|*.numbers|*.keynote|.env|.env.*|*/.env|*/.env.*|bin/*|dist/*|nas-governance)
      echo "public-boundary: forbidden tracked artifact: $path" >&2
      failed=1
      ;;
  esac
done < <(git ls-files -z)

# Construct signatures in fragments so this script does not match itself.
private_key_pattern='BEGIN [A-Z ]*PRIVATE'' KEY'
github_token_pattern='gh[pousr]_[A-Za-z0-9]{20,}'
aws_key_pattern='AKIA[0-9A-Z]{16}'
slack_token_pattern='xox[baprs]-[A-Za-z0-9-]{10,}'
local_user_path_pattern='/(Users|home)/[^/[:space:]]+/'
combined_pattern="${private_key_pattern}|${github_token_pattern}|${aws_key_pattern}|${slack_token_pattern}|${local_user_path_pattern}"

if git grep -nI -E "$combined_pattern" -- . ':!scripts/check-public-boundary.sh'; then
  echo "public-boundary: possible credential or workstation-specific path found" >&2
  failed=1
fi

if (( failed != 0 )); then
  exit 1
fi

echo "public-boundary: tracked artifact and text checks passed"
