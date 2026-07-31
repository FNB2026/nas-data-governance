#!/usr/bin/env bash
# create-dmg.sh — Create a distributable DMG from the signed NDG.app.
#
# This script performs:
#   1. Stage .app + Applications symlink in a temporary DMG root
#   2. Create UDZO-compressed DMG with hdiutil
#   3. Sign the DMG with Developer ID (if available)
#   4. Verify DMG signature and integrity
#
# The DMG must be signed BEFORE notarization. After notarization,
# run notarize-macos-app.sh to staple the ticket.
#
# Usage:
#   ./scripts/release/create-dmg.sh
#   ./scripts/release/create-dmg.sh --app path/to/NDG.app --output dist/NDG-0.5.0-beta.1-macos.dmg
#   ./scripts/release/create-dmg.sh --ad-hoc   # skip DMG signing
#
# Environment variables:
#   SIGNING_IDENTITY  — codesign identity for DMG signing
#   APP_PATH          — path to signed .app bundle
#   DMG_OUTPUT        — output DMG path
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Defaults
APP_PATH="${APP_PATH:-}"
DMG_OUTPUT="${DMG_OUTPUT:-}"
SIGNING_IDENTITY="${SIGNING_IDENTITY:-}"
AD_HOC=false
VOLNAME="NDG"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --app) APP_PATH="$2"; shift 2 ;;
        --output) DMG_OUTPUT="$2"; shift 2 ;;
        --identity) SIGNING_IDENTITY="$2"; shift 2 ;;
        --ad-hoc) AD_HOC=true; shift ;;
        *) echo "create-dmg: unknown argument '$1'" >&2; exit 1 ;;
    esac
done

# Auto-detect .app path
if [[ -z "$APP_PATH" ]]; then
    APP_PATH="$ROOT/cmd/ndg-desktop/build/bin/NDG.app"
fi

if [[ ! -d "$APP_PATH" ]]; then
    echo "create-dmg: FAIL — .app not found at $APP_PATH" >&2
    echo "  Run 'make desktop-build' and 'make desktop-sign' first." >&2
    exit 1
fi

# Read version for DMG filename
VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION" 2>/dev/null || echo "unknown")"

# Default output path
if [[ -z "$DMG_OUTPUT" ]]; then
    mkdir -p "$ROOT/dist"
    DMG_OUTPUT="$ROOT/dist/NDG-${VERSION}-macos.dmg"
fi

echo "create-dmg: APP = $APP_PATH"
echo "create-dmg: VERSION = $VERSION"
echo "create-dmg: DMG = $DMG_OUTPUT"

# --- Determine signing identity ---
if [[ "$AD_HOC" == "false" ]]; then
    if [[ -z "$SIGNING_IDENTITY" ]]; then
        SIGNING_IDENTITY="$(security find-identity -v -p codesigning 2>/dev/null \
            | grep 'Developer ID Application' \
            | head -1 \
            | sed 's/.*\) "\(.*\)"$/\1/' \
            || echo "")"
    fi

    if [[ -z "$SIGNING_IDENTITY" ]]; then
        echo "create-dmg: WARNING — no Developer ID Application certificate found" >&2
        echo "  DMG will be created but NOT signed. Notarization will fail." >&2
        echo "  Use --ad-hoc to suppress this warning for local testing." >&2
        AD_HOC=true
    fi
fi

# --- Stage DMG root ---
DMG_ROOT="$(mktemp -d)"
trap 'rm -rf "$DMG_ROOT"' EXIT

echo "create-dmg: staging DMG root..."
ditto "$APP_PATH" "$DMG_ROOT/NDG.app"
ln -s /Applications "$DMG_ROOT/Applications"

# --- Create DMG ---
echo "create-dmg: creating DMG..."
# Remove existing DMG if present
rm -f "$DMG_OUTPUT"

hdiutil create \
    -volname "$VOLNAME" \
    -srcfolder "$DMG_ROOT" \
    -format UDZO \
    -ov \
    "$DMG_OUTPUT" 2>&1

if [[ ! -f "$DMG_OUTPUT" ]]; then
    echo "create-dmg: FAIL — DMG was not created" >&2
    exit 1
fi

echo "create-dmg: DMG created ($(du -h "$DMG_OUTPUT" | cut -f1))"

# --- Sign DMG ---
if [[ "$AD_HOC" == "false" && -n "$SIGNING_IDENTITY" ]]; then
    echo "create-dmg: signing DMG with Developer ID..."
    codesign --force --timestamp --sign "$SIGNING_IDENTITY" "$DMG_OUTPUT"

    echo "create-dmg: verifying DMG signature..."
    if codesign --verify --verbose=4 "$DMG_OUTPUT" 2>&1; then
        echo "create-dmg: OK — DMG signature verified"
    else
        echo "create-dmg: FAIL — DMG signature verification failed" >&2
        exit 1
    fi
else
    echo "create-dmg: SKIP — DMG not signed (ad-hoc mode)"
fi

# --- Verify DMG integrity ---
echo "create-dmg: verifying DMG integrity..."
if hdiutil verify "$DMG_OUTPUT" 2>&1 | tail -1 | grep -q "hdiutil: verify"; then
    echo "create-dmg: OK — DMG integrity verified"
fi

# --- Record SHA256 ---
SHA256="$(shasum -a 256 "$DMG_OUTPUT" | cut -d' ' -f1)"
echo "create-dmg: SHA256 = $SHA256"

# Write checksum file
echo "$SHA256  $(basename "$DMG_OUTPUT")" > "${DMG_OUTPUT}.sha256"

echo ""
echo "create-dmg: done"
echo "  DMG: $DMG_OUTPUT"
echo "  SHA256: $SHA256"
