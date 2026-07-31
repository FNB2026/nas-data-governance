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

# ---------------------------------------------------------------------------
# B3: Validate SBOM JSON output — both files must be valid JSON with
# required top-level fields (CycloneDX: components; SPDX: packages).
# ---------------------------------------------------------------------------
validate_sbom_output() {
    local dir="$1"
    local cyclonedx_file="$dir/sbom.cyclonedx.json"
    local spdx_file="$dir/sbom.spdx.json"

    # Check files exist
    if [[ ! -f "$cyclonedx_file" ]]; then
        echo "generate-sbom: FAIL — CycloneDX SBOM file not found: $cyclonedx_file" >&2
        exit 1
    fi
    if [[ ! -f "$spdx_file" ]]; then
        echo "generate-sbom: FAIL — SPDX SBOM file not found: $spdx_file" >&2
        exit 1
    fi

    # Validate JSON syntax (jq will fail on malformed JSON)
    if ! jq . "$cyclonedx_file" >/dev/null 2>&1; then
        echo "generate-sbom: FAIL — CycloneDX SBOM is not valid JSON: $cyclonedx_file" >&2
        exit 1
    fi
    if ! jq . "$spdx_file" >/dev/null 2>&1; then
        echo "generate-sbom: FAIL — SPDX SBOM is not valid JSON: $spdx_file" >&2
        exit 1
    fi

    # Validate CycloneDX has non-empty components array
    local cyclonedx_components
    cyclonedx_components="$(jq '.components | length' "$cyclonedx_file" 2>/dev/null || echo "0")"
    if [[ "$cyclonedx_components" == "0" || "$cyclonedx_components" == "null" ]]; then
        echo "generate-sbom: FAIL — CycloneDX SBOM has no components" >&2
        exit 1
    fi

    # Validate SPDX has non-empty packages array
    local spdx_packages
    spdx_packages="$(jq '.packages | length' "$spdx_file" 2>/dev/null || echo "0")"
    if [[ "$spdx_packages" == "0" || "$spdx_packages" == "null" ]]; then
        echo "generate-sbom: FAIL — SPDX SBOM has no packages" >&2
        exit 1
    fi

    echo "generate-sbom: OK — SBOM validation passed"
    echo "  CycloneDX components: $cyclonedx_components"
    echo "  SPDX packages:        $spdx_packages"
}

echo "generate-sbom: generating SBOM for NDG..."

# ---------------------------------------------------------------------------
# Method 1: Use syft if available (covers both Go and npm in one scan)
# ---------------------------------------------------------------------------
if command -v syft >/dev/null 2>&1; then
    echo "generate-sbom: using syft (found in PATH)"

    # B3: No fail-open — syft errors must propagate.
    # Generate CycloneDX format
    if ! syft dir "$ROOT" \
        --output cyclonedx-json="$OUTPUT_DIR/sbom.cyclonedx.json" \
        --exclude '**/.git/**' \
        --exclude '**/node_modules/**' \
        --exclude '**/build/**' \
        --exclude '**/dist/**'; then
        echo "generate-sbom: FAIL — syft failed to generate CycloneDX SBOM" >&2
        exit 1
    fi

    # Generate SPDX format
    if ! syft dir "$ROOT" \
        --output spdx-json="$OUTPUT_DIR/sbom.spdx.json" \
        --exclude '**/.git/**' \
        --exclude '**/node_modules/**' \
        --exclude '**/build/**' \
        --exclude '**/dist/**'; then
        echo "generate-sbom: FAIL — syft failed to generate SPDX SBOM" >&2
        exit 1
    fi

    if [[ -f "$OUTPUT_DIR/sbom.cyclonedx.json" && -f "$OUTPUT_DIR/sbom.spdx.json" ]]; then
        # B3: Validate JSON output before declaring success
        validate_sbom_output "$OUTPUT_DIR"
        echo "generate-sbom: OK — SBOM generated via syft"
        echo "  CycloneDX: $OUTPUT_DIR/sbom.cyclonedx.json"
        echo "  SPDX:      $OUTPUT_DIR/sbom.spdx.json"
        exit 0
    fi
    echo "generate-sbom: FAIL — syft scan did not produce expected output files" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Method 2: Manual generation using Go tools + npm
# Install syft via curl if not present (for CI environments)
# ---------------------------------------------------------------------------
echo "generate-sbom: installing syft..."

SYFT_VERSION="v1.20.0"

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

# B7: Syft official releases use an aggregate checksums file, not per-file .sha256.
SYFT_ARCHIVE="$(basename "$SYFT_URL")"
SYFT_CHECKSUMS_URL="https://github.com/anchore/syft/releases/download/${SYFT_VERSION}/syft_${SYFT_VERSION#v}_checksums.txt"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# B3/B7: Download syft and verify SHA256 checksum from official aggregate file.
curl --fail --location --silent --show-error --output "$TMP_DIR/syft.tar.gz" "$SYFT_URL"

# Download the official aggregate checksums file
curl --fail --location --silent --show-error --output "$TMP_DIR/syft_checksums.txt" "$SYFT_CHECKSUMS_URL"

# B7: Extract the hash for our specific archive from the aggregate file.
# The file format is: "<sha256_hash>  <filename>"
EXPECTED_HASH="$(grep "  ${SYFT_ARCHIVE}$" "$TMP_DIR/syft_checksums.txt" | awk '{print $1}')"
ACTUAL_HASH="$(shasum -a 256 "$TMP_DIR/syft.tar.gz" | awk '{print $1}')"

if [[ -z "$EXPECTED_HASH" ]]; then
    echo "generate-sbom: FAIL — syft archive '$SYFT_ARCHIVE' not found in official checksums file" >&2
    exit 1
fi

if [[ "$EXPECTED_HASH" != "$ACTUAL_HASH" ]]; then
    echo "generate-sbom: FAIL — syft checksum mismatch" >&2
    echo "  Expected: $EXPECTED_HASH" >&2
    echo "  Actual:   $ACTUAL_HASH" >&2
    exit 1
fi
echo "generate-sbom: OK — syft checksum verified"

tar -xzf "$TMP_DIR/syft.tar.gz" -C "$TMP_DIR" syft
chmod +x "$TMP_DIR/syft"

echo "generate-sbom: running syft..."

# B3: No fail-open — syft errors must propagate.
# Generate CycloneDX format
if ! "$TMP_DIR/syft" dir "$ROOT" \
    --output cyclonedx-json="$OUTPUT_DIR/sbom.cyclonedx.json" \
    --exclude '**/.git/**' \
    --exclude '**/node_modules/**' \
    --exclude '**/build/**' \
    --exclude '**/dist/**'; then
    echo "generate-sbom: FAIL — syft failed to generate CycloneDX SBOM" >&2
    exit 1
fi

# Generate SPDX format
if ! "$TMP_DIR/syft" dir "$ROOT" \
    --output spdx-json="$OUTPUT_DIR/sbom.spdx.json" \
    --exclude '**/.git/**' \
    --exclude '**/node_modules/**' \
    --exclude '**/build/**' \
    --exclude '**/dist/**'; then
    echo "generate-sbom: FAIL — syft failed to generate SPDX SBOM" >&2
    exit 1
fi

# B3: Validate JSON output structure before declaring success
validate_sbom_output "$OUTPUT_DIR"

echo "generate-sbom: OK — SBOM generated"
echo "  CycloneDX: $OUTPUT_DIR/sbom.cyclonedx.json"
echo "  SPDX:      $OUTPUT_DIR/sbom.spdx.json"
