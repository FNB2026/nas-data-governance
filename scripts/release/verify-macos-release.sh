#!/usr/bin/env bash
# verify-macos-release.sh — Comprehensive verification of the macOS release
# artifacts (signed .app, signed/notarized/stapled DMG).
#
# This script performs:
#   1. Verify .app code signature (codesign --verify)
#   2. Verify .app Hardened Runtime (codesign -d)
#   3. Verify .app entitlements (codesign -d --entitlements)
#   4. Verify DMG signature (codesign --verify)
#   5. Verify DMG notarization staple (xcrun stapler validate)
#   6. Verify DMG Gatekeeper assessment (spctl --assess)
#   7. Verify DMG integrity (hdiutil verify)
#   8. Verify SHA256 checksum file
#   9. Print a summary of all verification results
#
# Usage:
#   ./scripts/release/verify-macos-release.sh
#   ./scripts/release/verify-macos-release.sh --app path/to/NDG.app --dmg path/to/NDG.dmg
#   ./scripts/release/verify-macos-release.sh --app-only   # skip DMG checks
#
# Environment variables:
#   APP_PATH  — path to .app bundle (default: auto-detect)
#   DMG_PATH  — path to DMG file (default: auto-detect)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Defaults
APP_PATH="${APP_PATH:-}"
DMG_PATH="${DMG_PATH:-}"
APP_ONLY=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --app) APP_PATH="$2"; shift 2 ;;
        --dmg) DMG_PATH="$2"; shift 2 ;;
        --app-only) APP_ONLY=true; shift ;;
        *) echo "verify: unknown argument '$1'" >&2; exit 1 ;;
    esac
done

# Auto-detect paths
if [[ -z "$APP_PATH" ]]; then
    APP_PATH="$ROOT/cmd/ndg-desktop/build/bin/NDG.app"
fi

if [[ -z "$DMG_PATH" && "$APP_ONLY" == "false" ]]; then
    VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION" 2>/dev/null || echo "unknown")"
    DMG_PATH="$ROOT/dist/NDG-${VERSION}-macos.dmg"
fi

# --- Counters ---
PASS=0
FAIL=0
SKIP=0

pass() { echo "  [PASS] $1"; PASS=$((PASS + 1)); }
fail() { echo "  [FAIL] $1" >&2; FAIL=$((FAIL + 1)); }
skip() { echo "  [SKIP] $1"; SKIP=$((SKIP + 1)); }

echo "========================================"
echo "  NDG macOS Release Verification"
echo "========================================"
echo ""

# ========== .app Verification ==========
echo "--- .app Bundle ---"
echo "  Path: $APP_PATH"

if [[ ! -d "$APP_PATH" ]]; then
    fail ".app not found at $APP_PATH"
    echo ""
    echo "  Run 'make desktop-build' and 'make desktop-sign' first." >&2
    exit 1
fi

# 1. codesign --verify (deep, strict)
echo ""
echo "  [1/9] Code signature verification..."
if codesign --verify --deep --strict --verbose=2 "$APP_PATH" 2>&1; then
    pass "codesign --verify --deep --strict"
else
    fail "codesign --verify --deep --strict"
fi

# 2. Hardened Runtime check
echo ""
echo "  [2/9] Hardened Runtime enabled..."
FLAGS="$(codesign -dvv "$APP_PATH" 2>&1 | grep 'CodeDirectory' || true)"
if echo "$FLAGS" | grep -q 'runtime'; then
    pass "Hardened Runtime is enabled"
else
    fail "Hardened Runtime is NOT enabled (missing 'runtime' flag)"
fi

# 3. Entitlements check
echo ""
echo "  [3/9] Entitlements present..."
ENTITLEMENTS_OUTPUT="$(codesign -d --entitlements - "$APP_PATH" 2>/dev/null || true)"
if echo "$ENTITLEMENTS_OUTPUT" | grep -q 'com.apple.security.cs.allow-jit'; then
    pass "JIT entitlement present (required for WebKit)"
else
    fail "JIT entitlement missing (required for WebKit)"
fi

if echo "$ENTITLEMENTS_OUTPUT" | grep -q 'com.apple.security.cs.allow-unsigned-executable-memory'; then
    pass "Unsigned executable memory entitlement present"
else
    fail "Unsigned executable memory entitlement missing"
fi

# 4. Team Identifier check (skip for ad-hoc)
echo ""
echo "  [4/9] Team Identifier..."
TEAM_ID="$(codesign -dvv "$APP_PATH" 2>&1 | grep 'TeamIdentifier' | awk '{print $2}' || true)"
if [[ -n "$TEAM_ID" && "$TEAM_ID" != "not set" ]]; then
    pass "Team Identifier = $TEAM_ID"
else
    echo "  Team Identifier not set (ad-hoc signature)"
    skip "Team Identifier (ad-hoc mode)"
fi

# 5. Bundle identifier check
echo ""
echo "  [5/9] Bundle Identifier..."
BUNDLE_ID="$(codesign -dvv "$APP_PATH" 2>&1 | grep 'Identifier' | head -1 | awk '{print $2}' || true)"
if [[ "$BUNDLE_ID" == "com.fnb.ndg" ]]; then
    pass "Bundle Identifier = $BUNDLE_ID"
else
    fail "Bundle Identifier = '$BUNDLE_ID' (expected 'com.fnb.ndg')"
fi

# ========== DMG Verification ==========
if [[ "$APP_ONLY" == "true" ]]; then
    echo ""
    echo "--- DMG ---"
    skip "DMG verification (--app-only mode)"
else
    echo ""
    echo "--- DMG ---"
    echo "  Path: $DMG_PATH"

    if [[ ! -f "$DMG_PATH" ]]; then
        fail "DMG not found at $DMG_PATH"
    else
        # 6. DMG signature verification
        echo ""
        echo "  [6/9] DMG code signature..."
        if codesign --verify --verbose=4 "$DMG_PATH" 2>&1; then
            pass "DMG code signature verified"
        else
            fail "DMG code signature verification failed"
        fi

        # 7. DMG notarization staple
        echo ""
        echo "  [7/9] DMG notarization staple..."
        if xcrun stapler validate "$DMG_PATH" 2>&1; then
            pass "DMG notarization staple validated"
        else
            fail "DMG notarization staple validation failed"
        fi

        # 8. Gatekeeper assessment
        echo ""
        echo "  [8/9] Gatekeeper assessment..."
        if spctl --assess --type open --context context:primary-signature --verbose "$DMG_PATH" 2>&1; then
            pass "Gatekeeper assessment accepted"
        else
            fail "Gatekeeper assessment rejected"
        fi

        # 9. DMG integrity + SHA256
        echo ""
        echo "  [9/9] DMG integrity and checksum..."
        if hdiutil verify "$DMG_PATH" >/dev/null 2>&1; then
            pass "DMG integrity verified (hdiutil verify)"
        else
            fail "DMG integrity verification failed"
        fi

        # SHA256 checksum file
        SHA256_FILE="${DMG_PATH}.sha256"
        if [[ -f "$SHA256_FILE" ]]; then
            EXPECTED="$(cut -d' ' -f1 < "$SHA256_FILE")"
            ACTUAL="$(shasum -a 256 "$DMG_PATH" | cut -d' ' -f1)"
            if [[ "$EXPECTED" == "$ACTUAL" ]]; then
                pass "SHA256 checksum matches ($ACTUAL)"
            else
                fail "SHA256 mismatch: expected=$EXPECTED actual=$ACTUAL"
            fi
        else
            skip "SHA256 checksum file not found (optional)"
        fi
    fi
fi

# ========== Summary ==========
echo ""
echo "========================================"
echo "  Verification Summary"
echo "========================================"
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "  SKIP: $SKIP"
echo ""

if [[ $FAIL -gt 0 ]]; then
    echo "verify: FAIL — $FAIL check(s) failed" >&2
    exit 1
fi

echo "verify: OK — all checks passed"
