#!/usr/bin/env bash
# notarize-macos-app.sh — Submit the signed DMG to Apple's notarization
# service, wait for approval, then staple the notarization ticket.
#
# This script performs:
#   1. Submit the DMG to Apple notarization via `xcrun notarytool submit`
#   2. Wait for the notarization to complete (--wait)
#   3. Staple the notarization ticket to the DMG via `xcrun stapler staple`
#   4. Verify the staple via `xcrun stapler validate`
#
# Prerequisites:
#   - DMG signed with Developer ID Application certificate
#   - Apple notarization credentials (one of):
#       a) Keychain profile (recommended):
#          xcrun notarytool store-credentials "NDG_NOTARY" \
#            --apple-id "you@example.com" \
#            --team-id "TEAMID" \
#            --password "app-specific-password"
#       b) Apple ID + app-specific password + team ID (via env vars)
#       c) App Store Connect API key (via env vars)
#
# Usage:
#   # Using keychain profile (recommended):
#   ./scripts/release/notarize-macos-app.sh --keychain-profile "NDG_NOTARY"
#
#   # Using Apple ID credentials:
#   ./scripts/release/notarize-macos-app.sh \
#     --apple-id "you@example.com" \
#     --team-id "TEAMID" \
#     --password "app-specific-password"
#
#   # Using API key:
#   ./scripts/release/notarize-macos-app.sh \
#     --key-id "KEYID" \
#     --key /path/to/AuthKey_KEYID.p8 \
#     --issuer "ISSUER_ID"
#
# Environment variables (override defaults):
#   DMG_PATH          — path to signed DMG (default: auto-detect)
#   NOTARY_PROFILE    — keychain profile name
#   APPLE_ID          — Apple ID email
#   APPLE_TEAM_ID     — Apple Team ID
#   APPLE_PASSWORD    — app-specific password
#   API_KEY_ID        — App Store Connect API key ID
#   API_KEY_PATH      — path to API key .p8 file
#   API_ISSUER_ID     — App Store Connect issuer ID
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Defaults
DMG_PATH="${DMG_PATH:-}"
NOTARY_PROFILE="${NOTARY_PROFILE:-}"
APPLE_ID="${APPLE_ID:-}"
APPLE_TEAM_ID="${APPLE_TEAM_ID:-}"
APPLE_PASSWORD="${APPLE_PASSWORD:-}"
API_KEY_ID="${API_KEY_ID:-}"
API_KEY_PATH="${API_KEY_PATH:-}"
API_ISSUER_ID="${API_ISSUER_ID:-}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dmg) DMG_PATH="$2"; shift 2 ;;
        --keychain-profile) NOTARY_PROFILE="$2"; shift 2 ;;
        --apple-id) APPLE_ID="$2"; shift 2 ;;
        --team-id) APPLE_TEAM_ID="$2"; shift 2 ;;
        --password) APPLE_PASSWORD="$2"; shift 2 ;;
        --key-id) API_KEY_ID="$2"; shift 2 ;;
        --key) API_KEY_PATH="$2"; shift 2 ;;
        --issuer) API_ISSUER_ID="$2"; shift 2 ;;
        *) echo "notarize: unknown argument '$1'" >&2; exit 1 ;;
    esac
done

# Auto-detect DMG path
if [[ -z "$DMG_PATH" ]]; then
    VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION" 2>/dev/null || echo "unknown")"
    DMG_PATH="$ROOT/dist/NDG-${VERSION}-macos.dmg"
fi

if [[ ! -f "$DMG_PATH" ]]; then
    echo "notarize: FAIL — DMG not found at $DMG_PATH" >&2
    echo "  Run 'make desktop-dmg' first." >&2
    exit 1
fi

echo "notarize: DMG = $DMG_PATH"

# --- Verify xcrun notarytool is available ---
if ! command -v xcrun >/dev/null 2>&1; then
    echo "notarize: FAIL — xcrun not found. Xcode Command Line Tools required." >&2
    exit 1
fi

# --- Build credential arguments for notarytool ---
CRED_ARGS=()

if [[ -n "$NOTARY_PROFILE" ]]; then
    # Method 1: Keychain profile (recommended)
    echo "notarize: using keychain profile '$NOTARY_PROFILE'"
    CRED_ARGS+=(--keychain-profile "$NOTARY_PROFILE")

elif [[ -n "$APPLE_ID" && -n "$APPLE_TEAM_ID" && -n "$APPLE_PASSWORD" ]]; then
    # Method 2: Apple ID + app-specific password + team ID
    echo "notarize: using Apple ID credentials"
    CRED_ARGS+=(--apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_PASSWORD")

elif [[ -n "$API_KEY_ID" && -n "$API_KEY_PATH" && -n "$API_ISSUER_ID" ]]; then
    # Method 3: App Store Connect API key
    echo "notarize: using API key credentials"
    CRED_ARGS+=(--key-id "$API_KEY_ID" --key "$API_KEY_PATH" --issuer "$API_ISSUER_ID")

else
    echo "notarize: FAIL — no notarization credentials provided" >&2
    echo "" >&2
    echo "  Configure one of the following:" >&2
    echo "" >&2
    echo "  1. Keychain profile (recommended):" >&2
    echo "     xcrun notarytool store-credentials 'NDG_NOTARY' \\" >&2
    echo "       --apple-id 'you@example.com' \\" >&2
    echo "       --team-id 'TEAMID' \\" >&2
    echo "       --password 'app-specific-password'" >&2
    echo "     Then: ./scripts/release/notarize-macos-app.sh --keychain-profile 'NDG_NOTARY'" >&2
    echo "" >&2
    echo "  2. Apple ID credentials:" >&2
    echo "     ./scripts/release/notarize-macos-app.sh \\" >&2
    echo "       --apple-id 'you@example.com' --team-id 'TEAMID' \\" >&2
    echo "       --password 'app-specific-password'" >&2
    echo "" >&2
    echo "  3. API key:" >&2
    echo "     ./scripts/release/notarize-macos-app.sh \\" >&2
    echo "       --key-id 'KEYID' --key /path/to/AuthKey.p8 --issuer 'ISSUER_ID'" >&2
    exit 1
fi

# --- Step 1: Submit DMG for notarization ---
echo ""
echo "notarize: submitting DMG to Apple notarization service..."
SUBMIT_OUTPUT="$(xcrun notarytool submit "$DMG_PATH" "${CRED_ARGS[@]}" 2>&1)"
echo "$SUBMIT_OUTPUT"

# Extract submission ID
SUBMISSION_ID="$(echo "$SUBMIT_OUTPUT" | grep -E '^id:' | awk '{print $2}')"
if [[ -z "$SUBMISSION_ID" ]]; then
    echo "notarize: FAIL — could not extract submission ID from notarytool output" >&2
    exit 1
fi

echo "notarize: submission ID = $SUBMISSION_ID"

# --- Step 2: Wait for notarization to complete ---
echo ""
echo "notarize: waiting for notarization to complete..."
echo "  (this typically takes 2-15 minutes; timeout: 60 minutes)"

# Use --wait to block until completion
set +e
xcrun notarytool wait "$SUBMISSION_ID" "${CRED_ARGS[@]}" --timeout 60m 2>&1
WAIT_EXIT_CODE=$?
set -e

if [[ $WAIT_EXIT_CODE -ne 0 ]]; then
    echo "notarize: FAIL — notarization did not complete within timeout" >&2
    echo "  Check status: xcrun notarytool info $SUBMISSION_ID ${CRED_ARGS[*]//[^[:print:]]/}" >&2
    exit 1
fi

# --- Step 3: Check notarization status ---
echo ""
echo "notarize: checking notarization result..."
NOTARY_INFO="$(xcrun notarytool info "$SUBMISSION_ID" "${CRED_ARGS[@]}" 2>&1)"
echo "$NOTARY_INFO"

if ! echo "$NOTARY_INFO" | grep -q 'Accepted'; then
    echo "notarize: FAIL — notarization was not accepted" >&2
    echo "  Fetch the log for details:" >&2
    echo "  xcrun notarytool log $SUBMISSION_ID ${CRED_ARGS[*]//[^[:print:]]/}" >&2
    exit 1
fi

echo "notarize: OK — notarization accepted"

# --- Step 4: Staple the notarization ticket to the DMG ---
echo ""
echo "notarize: stapling notarization ticket to DMG..."
xcrun stapler staple "$DMG_PATH" 2>&1
echo "notarize: staple completed"

# --- Step 5: Verify the staple ---
echo ""
echo "notarize: verifying staple..."
if xcrun stapler validate "$DMG_PATH" 2>&1; then
    echo "notarize: OK — staple validated"
else
    echo "notarize: FAIL — staple validation failed" >&2
    exit 1
fi

echo ""
echo "notarize: done"
echo "  DMG: $DMG_PATH"
echo "  Submission ID: $SUBMISSION_ID"
echo "  Status: Accepted & Stapled"
