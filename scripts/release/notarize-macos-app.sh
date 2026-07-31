#!/usr/bin/env bash
# notarize-macos-app.sh — Submit the signed DMG to Apple's notarization
# service, wait for approval, then staple the notarization ticket.
#
# This script performs:
#   1. Submit the DMG to Apple notarization via `xcrun notarytool submit`
#   2. Wait for the notarization to complete (notarytool wait)
#   3. Check the notarization result (must be Accepted)
#   4. Staple the notarization ticket to the DMG via `xcrun stapler staple`
#   5. Verify the staple via `xcrun stapler validate`
#   6. Generate the final SHA256 checksum (AFTER staple, because stapler
#      modifies the DMG file and invalidates any earlier checksum)
#
# Prerequisites:
#   - DMG signed with Developer ID Application certificate
#   - Apple notarization credentials (one of):
#       a) Keychain profile (recommended):
#          xcrun notarytool store-credentials "NDG_NOTARY" \
#            --apple-id "you@example.com" \
#            --team-id "TEAMID" \
#            --password "app-specific-password"
#       b) App Store Connect API key (for CI/automation):
#          Create a key at appstoreconnect.apple.com → Access → Keys
#
# Security: This script NEVER accepts or prints Apple ID passwords.
# Password-based notarization is intentionally removed to prevent
# credential leakage in logs or error messages.
#
# Usage:
#   # Using keychain profile (recommended for manual releases):
#   ./scripts/release/notarize-macos-app.sh --keychain-profile "NDG_NOTARY"
#
#   # Using API key (recommended for CI):
#   ./scripts/release/notarize-macos-app.sh \
#     --key-id "KEYID" \
#     --key /path/to/AuthKey_KEYID.p8 \
#     --issuer "ISSUER_ID"
#
# Environment variables (override defaults):
#   DMG_PATH          — path to signed DMG (default: auto-detect)
#   NOTARY_PROFILE    — keychain profile name
#   API_KEY_ID        — App Store Connect API key ID
#   API_KEY_PATH      — path to API key .p8 file
#   API_ISSUER_ID     — App Store Connect issuer ID
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Defaults
DMG_PATH="${DMG_PATH:-}"
NOTARY_PROFILE="${NOTARY_PROFILE:-}"
API_KEY_ID="${API_KEY_ID:-}"
API_KEY_PATH="${API_KEY_PATH:-}"
API_ISSUER_ID="${API_ISSUER_ID:-}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dmg) DMG_PATH="$2"; shift 2 ;;
        --keychain-profile) NOTARY_PROFILE="$2"; shift 2 ;;
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
# CRED_ARGS is NEVER printed or expanded in any output to prevent
# credential leakage. Only the credential method name is logged.
CRED_ARGS=()
CRED_METHOD=""

if [[ -n "$NOTARY_PROFILE" ]]; then
    # Method 1: Keychain profile (recommended for manual releases)
    CRED_METHOD="keychain-profile"
    CRED_ARGS+=(--keychain-profile "$NOTARY_PROFILE")

elif [[ -n "$API_KEY_ID" && -n "$API_KEY_PATH" && -n "$API_ISSUER_ID" ]]; then
    # Method 2: App Store Connect API key (recommended for CI)
    CRED_METHOD="api-key"
    CRED_ARGS+=(--key-id "$API_KEY_ID" --key "$API_KEY_PATH" --issuer "$API_ISSUER_ID")

else
    echo "notarize: FAIL — no notarization credentials provided" >&2
    echo "" >&2
    echo "  Configure one of the following:" >&2
    echo "" >&2
    echo "  1. Keychain profile (recommended for manual releases):" >&2
    echo "     xcrun notarytool store-credentials 'NDG_NOTARY' \\" >&2
    echo "       --apple-id 'you@example.com' \\" >&2
    echo "       --team-id 'TEAMID' \\" >&2
    echo "       --password 'app-specific-password'" >&2
    echo "     Then: ./scripts/release/notarize-macos-app.sh --keychain-profile 'NDG_NOTARY'" >&2
    echo "" >&2
    echo "  2. API key (recommended for CI):" >&2
    echo "     ./scripts/release/notarize-macos-app.sh \\" >&2
    echo "       --key-id 'KEYID' --key /path/to/AuthKey.p8 --issuer 'ISSUER_ID'" >&2
    echo "" >&2
    echo "  Note: Apple ID + password mode is intentionally removed to prevent" >&2
    echo "  credential leakage in logs. Use keychain profile or API key instead." >&2
    exit 1
fi

echo "notarize: credential method = $CRED_METHOD"

# --- Step 1: Submit DMG for notarization ---
echo ""
echo "notarize: submitting DMG to Apple notarization service..."
SUBMIT_OUTPUT="$(xcrun notarytool submit "$DMG_PATH" "${CRED_ARGS[@]}" 2>&1)"
echo "$SUBMIT_OUTPUT"

# Extract submission ID
SUBMISSION_ID="$(echo "$SUBMIT_OUTPUT" | grep -E '^id:' | awk '{print $2}')"
if [[ -z "$SUBMISSION_ID" ]]; then
    echo "notarize: FAIL — could not extract submission ID from notarytool output" >&2
    echo "  Check notarytool output above for errors." >&2
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
    echo "  Check status with: xcrun notarytool info $SUBMISSION_ID (use your credentials)" >&2
    exit 1
fi

# --- Step 3: Check notarization status ---
echo ""
echo "notarize: checking notarization result..."
NOTARY_INFO="$(xcrun notarytool info "$SUBMISSION_ID" "${CRED_ARGS[@]}" 2>&1)"
echo "$NOTARY_INFO"

if ! echo "$NOTARY_INFO" | grep -q 'Accepted'; then
    echo "notarize: FAIL — notarization was not accepted" >&2
    echo "  Fetch the log with: xcrun notarytool log $SUBMISSION_ID (use your credentials)" >&2
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

# --- Step 6: Verify DMG integrity after staple ---
echo ""
echo "notarize: verifying DMG integrity after staple..."
if hdiutil verify "$DMG_PATH" 2>&1; then
    echo "notarize: OK — DMG integrity verified after staple"
else
    echo "notarize: FAIL — DMG integrity verification failed after staple" >&2
    exit 1
fi

# --- Step 7: Generate final SHA256 (AFTER staple) ---
# Stapler modifies the DMG file, so the checksum MUST be generated
# after stapling, not before. This is the authoritative checksum.
echo ""
echo "notarize: generating final SHA256 checksum..."
SHA256="$(shasum -a 256 "$DMG_PATH" | cut -d' ' -f1)"
echo "$SHA256  $(basename "$DMG_PATH")" > "${DMG_PATH}.sha256"
echo "notarize: SHA256 = $SHA256"

echo ""
echo "notarize: done"
echo "  DMG: $DMG_PATH"
echo "  Submission ID: $SUBMISSION_ID"
echo "  Status: Accepted & Stapled"
echo "  SHA256: $SHA256"
