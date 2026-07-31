#!/usr/bin/env bash
# generate-sbom.sh — Generate Software Bill of Materials (SBOM) for NDG.
#
# Generates SBOM in CycloneDX and SPDX formats covering:
#   - Go module dependencies (go.mod)
#   - npm frontend dependencies (package-lock.json)
#
# Prerequisites:
#   - Go (for cyclonedx-gomod)
#   - Node.js (for npm)
#   - syft (installed automatically if not present)
#
# Usage:
#   ./scripts/release/generate-sbom.sh
#   ./scripts/release/generate-sbom.sh --output dist
#
# Environment variables:
#   OUTPUT_DIR — output directory (default: dist)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Defaults
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT/dist}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --output) OUTPUT_DIR="$2"; shift 2 ;;
        *) echo "generate-sbom: unknown argument '$1'" >&2; exit 1 ;;
    esac
done

mkdir -p "$OUTPUT_DIR"

echo "generate-sbom: generating SBOM for NDG..."

# ---------------------------------------------------------------------------
# Method 1: Use syft if available (covers both Go and npm in one scan)
# ---------------------------------------------------------------------------
if command -v syft >/dev/null 2>&1; then
    echo "generate-sbom: using syft (found in PATH)"

    # Generate CycloneDX format
    syft dir "$ROOT" \
        --output cyclonedx-json="$OUTPUT_DIR/sbom.cyclonedx.json" \
        --exclude '**/.git/**' \
        --exclude '**/node_modules/**' \
        --exclude '**/build/**' \
        --exclude '**/dist/**' 2>&1 || true

    # Generate SPDX format
    syft dir "$ROOT" \
        --output spdx-json="$OUTPUT_DIR/sbom.spdx.json" \
        --exclude '**/.git/**' \
        --exclude '**/node_modules/**' \
        --exclude '**/build/**' \
        --exclude '**/dist/**' 2>&1 || true

    if [[ -f "$OUTPUT_DIR/sbom.cyclonedx.json" && -f "$OUTPUT_DIR/sbom.spdx.json" ]]; then
        echo "generate-sbom: OK — SBOM generated via syft"
        echo "  CycloneDX: $OUTPUT_DIR/sbom.cyclonedx.json"
        echo "  SPDX:      $OUTPUT_DIR/sbom.spdx.json"
        exit 0
    fi
    echo "generate-sbom: syft scan incomplete, falling back to manual generation..."
fi

# ---------------------------------------------------------------------------
# Method 2: Manual generation using Go tools + npm
# Install syft via curl if not present (for CI environments)
# ---------------------------------------------------------------------------
echo "generate-sbom: installing syft..."

SYFT_VERSION="v1.18.2"
SYFT_URL="https://github.com/anchore/syft/releases/download/${SYFT_VERSION}/syft_${SYFT_VERSION#v}_linux_amd64.tar.gz"

# Detect platform
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "generate-sbom: unsupported architecture '$ARCH'" >&2; exit 1 ;;
esac

case "$OS" in
    darwin) SYFT_URL="https://github.com/anchore/syft/releases/download/${SYFT_VERSION}/syft_${SYFT_VERSION#v}_darwin_${ARCH}.tar.gz" ;;
    linux)  SYFT_URL="https://github.com/anchore/syft/releases/download/${SYFT_VERSION}/syft_${SYFT_VERSION#v}_linux_${ARCH}.tar.gz" ;;
    *) echo "generate-sbom: unsupported OS '$OS'" >&2; exit 1 ;;
esac

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

curl --fail --location --silent --show-error --output "$TMP_DIR/syft.tar.gz" "$SYFT_URL"
tar -xzf "$TMP_DIR/syft.tar.gz" -C "$TMP_DIR" syft
chmod +x "$TMP_DIR/syft"

echo "generate-sbom: running syft..."

# Generate CycloneDX format
"$TMP_DIR/syft" dir "$ROOT" \
    --output cyclonedx-json="$OUTPUT_DIR/sbom.cyclonedx.json" \
    --exclude '**/.git/**' \
    --exclude '**/node_modules/**' \
    --exclude '**/build/**' \
    --exclude '**/dist/**'

# Generate SPDX format
"$TMP_DIR/syft" dir "$ROOT" \
    --output spdx-json="$OUTPUT_DIR/sbom.spdx.json" \
    --exclude '**/.git/**' \
    --exclude '**/node_modules/**' \
    --exclude '**/build/**' \
    --exclude '**/dist/**'

# Verify outputs
if [[ ! -f "$OUTPUT_DIR/sbom.cyclonedx.json" ]]; then
    echo "generate-sbom: FAIL — CycloneDX SBOM was not generated" >&2
    exit 1
fi

if [[ ! -f "$OUTPUT_DIR/sbom.spdx.json" ]]; then
    echo "generate-sbom: FAIL — SPDX SBOM was not generated" >&2
    exit 1
fi

echo "generate-sbom: OK — SBOM generated"
echo "  CycloneDX: $OUTPUT_DIR/sbom.cyclonedx.json"
echo "  SPDX:      $OUTPUT_DIR/sbom.spdx.json"
