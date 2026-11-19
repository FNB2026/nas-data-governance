#!/usr/bin/env bash
# Builds a project database with the schema-11 code base and the SQLite driver
# that shipped with it, so the compatibility test opens a file an older build
# actually produced instead of one today's code invented.
#
# The commit is pinned: 3563831 is the last one before the governance batch
# schema landed, and it still requires modernc.org/sqlite v1.56.0. The layout it
# produces matches legacySchema11Layout in internal/store/schema_identity.go; the
# script fails loudly if that ever stops being true.
set -euo pipefail

OUT=${1:-/tmp/ndg-legacy/legacy.db}
LEGACY_COMMIT=${NDG_LEGACY_COMMIT:-356383115f43c5d75ed2827cf7520d3172ae055f}
EXPECTED_LAYOUT=${NDG_LEGACY_LAYOUT:-e177d2a1ec16d33cd96eed49a70082c8b25a10c5b6b72ca7e4c7e5138f811af5}

ROOT=$(git rev-parse --show-toplevel)
GENERATOR="$ROOT/scripts/qa/compatgen/main.go"
if [[ ! -f "$GENERATOR" ]]; then
  echo "make-legacy-database: generator missing at $GENERATOR" >&2
  exit 1
fi
if ! git -C "$ROOT" cat-file -e "$LEGACY_COMMIT^{commit}" 2>/dev/null; then
  echo "make-legacy-database: commit $LEGACY_COMMIT is not available (need full history)" >&2
  exit 1
fi

# Resolve the output path before any directory change, so the fixture always
# lands where the caller asked for it.
OUTPUT_DIR=$(dirname "$OUT")
mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR=$(cd "$OUTPUT_DIR" && pwd)
OUT="$OUTPUT_DIR/$(basename "$OUT")"

# Never destroy an existing database, and never write inside the checkout.
if [[ -e "$OUT" ]]; then
  echo "make-legacy-database: refusing to overwrite the existing $OUT" >&2
  exit 1
fi
case "$OUT" in
  "$ROOT"/*)
    echo "make-legacy-database: refusing to write inside the repository at $OUT" >&2
    exit 1
    ;;
esac

WORK=$(mktemp -d)
cleanup() {
  git -C "$ROOT" worktree remove --force "$WORK/tree" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

git -C "$ROOT" worktree add --detach "$WORK/tree" "$LEGACY_COMMIT" >/dev/null
mkdir -p "$WORK/tree/cmd/compatgen"
cp "$GENERATOR" "$WORK/tree/cmd/compatgen/main.go"

RESULT=$(cd "$WORK/tree" && go run ./cmd/compatgen "$OUT")
echo "$RESULT"

ACTUAL=$(printf '%s' "$RESULT" | sed -n 's/.*"layout":"\([0-9a-f]*\)".*/\1/p')
if [[ "$ACTUAL" != "$EXPECTED_LAYOUT" ]]; then
  echo "make-legacy-database: produced layout $ACTUAL, expected $EXPECTED_LAYOUT" >&2
  exit 1
fi
echo "make-legacy-database: layout matches legacySchema11Layout"
