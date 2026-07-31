#!/usr/bin/env bash
# sign-macos-app.sh — Sign the NDG .app bundle with Developer ID and
# Hardened Runtime, preparing it for notarization.
#
# This script performs:
#   1. Clear extended attributes (xattr -cr)
#   2. codesign with --options runtime (Hardened Runtime)
#   3. Verify the signature
#
# Prerequisites:
#   - Developer ID Application certificate in keychain
#   - entitlements.plist at cmd/ndg-desktop/build/darwin/entitlements.plist
#
# Usage:
#   ./scripts/release/sign-macos-app.sh
#   ./scripts/release/sign-macos-app.sh --identity "Developer ID Application: Your Name (TEAM_ID)"
#   ./scripts/release/sign-macos-app.sh --ad-hoc   # local testing only
#
# Environment variables (override defaults):
#   SIGNING_IDENTITY  — codesign identity name
#   APP_PATH          — path to .app bundle (default: auto-detect)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENTITLEMENTS="$ROOT/cmd/ndg-desktop/build/darwin/entitlements.plist"

# Defaults
SIGNING_IDENTITY="${SIGNING_IDENTITY:-}"
APP_PATH="${APP_PATH:-}"
AD_HOC=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --identity) SIGNING_IDENTITY="$2"; shift 2 ;;
        --ad-hoc) AD_HOC=true; shift ;;
        --app) APP_PATH="$2"; shift 2 ;;
        *) echo "sign-macos: unknown argument '$1'" >&2; exit 1 ;;
    esac
done

# Auto-detect .app path if not specified
if [[ -z "$APP_PATH" ]]; then
    APP_PATH="$ROOT/cmd/ndg-desktop/build/bin/NDG.app"
fi

if [[ ! -d "$APP_PATH" ]]; then
    echo "sign-macos: FAIL — .app not found at $APP_PATH" >&2
    echo "  Run 'make desktop-build' first." >&2
    exit 1
fi

if [[ ! -f "$ENTITLEMENTS" ]]; then
    echo "sign-macos: FAIL — entitlements not found at $ENTITLEMENTS" >&2
    exit 1
fi

echo "sign-macos: APP = $APP_PATH"
echo "sign-macos: ENTITLEMENTS = $ENTITLEMENTS"

# --- Determine signing identity ---
if [[ "$AD_HOC" == "true" ]]; then
    SIGNING_IDENTITY="-"
    echo "sign-macos: using ad-hoc signing (local testing only, NOT for distribution)"
elif [[ -z "$SIGNING_IDENTITY" ]]; then
    # Auto-detect Developer ID Application certificate
    # Parse: '  1) ABCDE12345 "Developer ID Application: Your Name (TEAMID)"'
    # Extract the quoted string using awk with double-quote delimiter.
    SIGNING_IDENTITY="$(security find-identity -v -p codesigning 2>/dev/null \
        | awk -F '"' '/Developer ID Application/ { print $2; exit }' \
        || echo "")"

    if [[ -z "$SIGNING_IDENTITY" ]]; then
        echo "sign-macos: FAIL — no 'Developer ID Application' certificate found in keychain" >&2
        echo "  Options:" >&2
        echo "    1. Obtain a Developer ID Application certificate from Apple Developer Program" >&2
        echo "    2. Use --ad-hoc for local testing (NOT for distribution)" >&2
        echo "    3. Specify identity with --identity or SIGNING_IDENTITY env var" >&2
        echo "" >&2
        echo "  Available codesigning identities:" >&2
        security find-identity -v -p codesigning 2>&1 | sed 's/^/    /' >&2
        exit 1
    fi
fi

echo "sign-macos: SIGNING_IDENTITY = $SIGNING_IDENTITY"

# --- Step 1: Clear extended attributes ---
echo "sign-macos: clearing extended attributes..."
xattr -cr "$APP_PATH"

# --- Step 2: codesign with Hardened Runtime ---
echo "sign-macos: signing with Hardened Runtime..."

if [[ "$AD_HOC" == "true" ]]; then
    # Ad-hoc: no timestamp, no Developer ID, but still enable Hardened Runtime
    codesign --force \
        --deep \
        --options runtime \
        --entitlements "$ENTITLEMENTS" \
        --sign "$SIGNING_IDENTITY" \
        "$APP_PATH"
else
    codesign --force \
        --deep \
        --options runtime \
        --entitlements "$ENTITLEMENTS" \
        --timestamp \
        --sign "$SIGNING_IDENTITY" \
        "$APP_PATH"
fi

echo "sign-macos: codesign completed"

# --- Step 3: Verify signature ---
echo "sign-macos: verifying signature..."
if codesign --verify --deep --strict --verbose=2 "$APP_PATH" 2>&1; then
    echo "sign-macos: OK — signature verified"
else
    echo "sign-macos: FAIL — signature verification failed" >&2
    exit 1
fi

# --- Display signing details ---
echo ""
echo "sign-macos: signing details:"
codesign -dvv "$APP_PATH" 2>&1 | grep -E '(Identifier|Authority|TeamIdentifier|Flags|Format|CodeDirectory)' | sed 's/^/  /'

echo ""
echo "sign-macos: done"
