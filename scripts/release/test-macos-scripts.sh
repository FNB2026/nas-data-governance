#!/usr/bin/env bash
# test-macos-scripts.sh — Mock-based behavior tests for R2 macOS release scripts.
#
# These tests verify script BEHAVIOR without requiring:
#   - A built .app bundle
#   - Developer ID certificates
#   - Apple notarization credentials
#   - Actual codesign/hdiutil/xcrun invocations
#
# Test coverage:
#   1. Developer ID output parsing (awk -F'"' not sed)
#   2. Bundle ID parsing (cut -d= not awk space)
#   3. Team ID parsing (cut -d= not awk space)
#   4. Password not accepted by notarize script
#   5. hdiutil verify failure causes exit in create-dmg
#   6. SHA256 deferred in production create-dmg mode
#   7. Entitlements file contains only minimal permissions
#   8. Makefile desktop-release depends on version-check
#   9. Makefile desktop-verify supports APP_ONLY=true
#
# Usage:
#   ./scripts/release/test-macos-scripts.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

PASS=0
FAIL=0

pass() { echo "  PASS — $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL — $1" >&2; FAIL=$((FAIL + 1)); }

echo "=== test-macos-scripts: R2 macOS release script behavior tests ==="
echo ""

# ---------------------------------------------------------------------------
# Test 1: Developer ID output parsing
# Verify that awk -F'"' correctly parses security find-identity output.
# ---------------------------------------------------------------------------
echo "--- Test 1: Developer ID output parsing ---"

MOCK_SECURITY_OUTPUT='  1) ABCDE12345 "Developer ID Application: Your Name (TEAMID)"
  2) GFHIJ67890 "Developer ID Installer: Your Name (TEAMID)"
  3) KLMNO11111 "Mac Developer: Your Name (TEAMID)"'

PARSED_ID="$(echo "$MOCK_SECURITY_OUTPUT" | awk -F '"' '/Developer ID Application/ { print $2; exit }')"

if [[ "$PARSED_ID" == "Developer ID Application: Your Name (TEAMID)" ]]; then
    pass "awk -F'\"' correctly parses Developer ID Application certificate"
else
    fail "awk parsing failed: got '$PARSED_ID'"
fi

# Verify old sed expression would have failed
if echo "$MOCK_SECURITY_OUTPUT" | sed 's/.*\) "\(.*\)"$/\1/' >/dev/null 2>&1; then
    # If sed doesn't error, check if the result is correct
    SED_RESULT="$(echo "$MOCK_SECURITY_OUTPUT" | grep 'Developer ID Application' | head -1 | sed 's/.*\) "\(.*\)"$/\1/' 2>/dev/null || echo "SED_ERROR")"
    if [[ "$SED_RESULT" == "SED_ERROR" || -z "$SED_RESULT" ]]; then
        pass "old sed expression is confirmed broken (would not produce correct result)"
    else
        fail "old sed expression unexpectedly works — test assumption invalid"
    fi
else
    pass "old sed expression is confirmed broken (sed exits non-zero)"
fi

echo ""

# ---------------------------------------------------------------------------
# Test 2: Bundle ID parsing from codesign -dvv output
# codesign -dvv outputs: Identifier=com.fnb.ndg
# Must use cut -d= -f2, NOT awk '{print $2}' (no spaces in output).
# ---------------------------------------------------------------------------
echo "--- Test 2: Bundle ID parsing ---"

MOCK_CODESIGN_DVV="Executable=/path/to/NDG.app/Contents/MacOS/NDG
Identifier=com.fnb.ndg
Format=app bundle with Mach-O thin (arm64)
CodeDirectory v=20400 ... flags=0x10000(runtime) ...
TeamIdentifier=ABCDE12345
Identifier=com.fnb.ndg"

# Test the correct parsing (cut -d= -f2)
PARSED_BUNDLE_ID="$(echo "$MOCK_CODESIGN_DVV" | grep '^Identifier' | head -1 | cut -d= -f2)"

if [[ "$PARSED_BUNDLE_ID" == "com.fnb.ndg" ]]; then
    pass "cut -d= -f2 correctly parses Bundle ID"
else
    fail "cut -d= -f2 failed: got '$PARSED_BUNDLE_ID'"
fi

# Verify old awk '{print $2}' would have failed
OLD_AWK_RESULT="$(echo "$MOCK_CODESIGN_DVV" | grep '^Identifier' | head -1 | awk '{print $2}')"
if [[ -z "$OLD_AWK_RESULT" ]]; then
    pass "old awk '{print \$2}' confirmed to return empty (no spaces in Identifier= line)"
else
    fail "old awk '{print \$2}' unexpectedly returned '$OLD_AWK_RESULT'"
fi

echo ""

# ---------------------------------------------------------------------------
# Test 3: Team ID parsing from codesign -dvv output
# codesign -dvv outputs: TeamIdentifier=ABCDE12345
# ---------------------------------------------------------------------------
echo "--- Test 3: Team ID parsing ---"

PARSED_TEAM_ID="$(echo "$MOCK_CODESIGN_DVV" | grep '^TeamIdentifier' | head -1 | cut -d= -f2)"

if [[ "$PARSED_TEAM_ID" == "ABCDE12345" ]]; then
    pass "cut -d= -f2 correctly parses Team ID"
else
    fail "cut -d= -f2 failed: got '$PARSED_TEAM_ID'"
fi

# Verify old awk would have failed
OLD_TEAM_AWK="$(echo "$MOCK_CODESIGN_DVV" | grep '^TeamIdentifier' | head -1 | awk '{print $2}')"
if [[ -z "$OLD_TEAM_AWK" ]]; then
    pass "old awk '{print \$2}' confirmed to return empty for TeamIdentifier"
else
    fail "old awk '{print \$2}' unexpectedly returned '$OLD_TEAM_AWK'"
fi

echo ""

# ---------------------------------------------------------------------------
# Test 4: notarize-macos-app.sh rejects --password / --apple-id
# The script must not accept Apple ID + password mode.
# ---------------------------------------------------------------------------
echo "--- Test 4: notarize script rejects password mode ---"

# Verify --apple-id and --password are NOT in the script's case statement
if grep -q '\-\-apple-id\|\-\-password' "$ROOT/scripts/release/notarize-macos-app.sh"; then
    # Check if they're in case statement (accepted args) vs comments
    if grep -E '^\s+--apple-id\)|--password\)' "$ROOT/scripts/release/notarize-macos-app.sh" >/dev/null 2>&1; then
        fail "notarize script still accepts --apple-id or --password as arguments"
    else
        pass "notarize script mentions --apple-id/--password only in comments/help text"
    fi
else
    pass "notarize script does not reference --apple-id or --password at all"
fi

# Verify CRED_ARGS is never expanded in echo/printf statements
if grep -nE 'echo.*\$\{?CRED_ARGS' "$ROOT/scripts/release/notarize-macos-app.sh" >/dev/null 2>&1; then
    fail "notarize script expands CRED_ARGS in echo statement (credential leak risk)"
else
    pass "notarize script never expands CRED_ARGS in output"
fi

# Verify only keychain-profile and api-key methods are accepted
if grep -q '\-\-keychain-profile' "$ROOT/scripts/release/notarize-macos-app.sh" && \
   grep -q '\-\-key-id' "$ROOT/scripts/release/notarize-macos-app.sh"; then
    pass "notarize script supports keychain-profile and api-key methods"
else
    fail "notarize script missing required credential methods"
fi

echo ""

# ---------------------------------------------------------------------------
# Test 5: create-dmg.sh hdiutil verify failure causes exit
# The script must exit non-zero when hdiutil verify fails.
# ---------------------------------------------------------------------------
echo "--- Test 5: hdiutil verify failure exits in create-dmg.sh ---"

# Extract the hdiutil verify block and check for else/exit
DMG_VERIFY_BLOCK="$(sed -n '/verifying DMG integrity/,/^fi/p' "$ROOT/scripts/release/create-dmg.sh")"

if echo "$DMG_VERIFY_BLOCK" | grep -q 'exit 1'; then
    pass "create-dmg.sh has exit 1 in hdiutil verify failure branch"
else
    fail "create-dmg.sh missing exit 1 on hdiutil verify failure"
fi

# Also check notarize script has the same pattern
NOTARY_VERIFY_BLOCK="$(sed -n '/DMG integrity after staple/,/^fi/p' "$ROOT/scripts/release/notarize-macos-app.sh")"
if echo "$NOTARY_VERIFY_BLOCK" | grep -q 'exit 1'; then
    pass "notarize-macos-app.sh has exit 1 in post-staple hdiutil verify failure branch"
else
    fail "notarize-macos-app.sh missing exit 1 on post-staple verify failure"
fi

# Check create-dmg.sh fails closed (no auto-downgrade to ad-hoc)
if grep -A3 'no Developer ID Application certificate found' "$ROOT/scripts/release/create-dmg.sh" | grep -q 'exit 1'; then
    pass "create-dmg.sh fails closed when no Developer ID (no auto-downgrade)"
else
    fail "create-dmg.sh auto-downgrades to ad-hoc instead of failing"
fi

# Verify AD_HOC=true is NOT set automatically (outside of --ad-hoc arg parsing)
# The only legitimate AD_HOC=true is in the --ad-hoc) case statement line.
AUTO_DOWNGRADE_LINES="$(grep -n 'AD_HOC=true' "$ROOT/scripts/release/create-dmg.sh" | grep -v 'AD_HOC=false' | grep -v 'AD_HOC="${AD_HOC' | grep -v '\-\-ad-hoc)' || true)"
if [[ -n "$AUTO_DOWNGRADE_LINES" ]]; then
    fail "create-dmg.sh sets AD_HOC=true automatically outside arg parsing: $AUTO_DOWNGRADE_LINES"
else
    pass "create-dmg.sh does not auto-set AD_HOC=true (only via --ad-hoc arg)"
fi

echo ""

# ---------------------------------------------------------------------------
# Test 6: SHA256 deferred in production create-dmg mode
# In production mode (not ad-hoc), create-dmg.sh must NOT generate SHA256.
# SHA256 is generated by notarize-macos-app.sh AFTER staple.
# ---------------------------------------------------------------------------
echo "--- Test 6: SHA256 deferred in production mode ---"

# Check create-dmg.sh generates SHA256 only in ad-hoc mode
if grep -A5 'Record SHA256' "$ROOT/scripts/release/create-dmg.sh" | grep -q 'AD_HOC.*true'; then
    pass "create-dmg.sh gates SHA256 generation behind AD_HOC=true"
else
    fail "create-dmg.sh does not properly gate SHA256 by ad-hoc mode"
fi

# Check create-dmg.sh removes stale SHA256 in production mode
if grep -q 'rm -f.*sha256' "$ROOT/scripts/release/create-dmg.sh"; then
    pass "create-dmg.sh removes stale SHA256 in production mode"
else
    fail "create-dmg.sh does not clean up stale SHA256 in production mode"
fi

# Check notarize-macos-app.sh generates SHA256 after staple
# Verify the script contains a SHA256 generation step with "AFTER staple" comment
if grep -q 'shasum' "$ROOT/scripts/release/notarize-macos-app.sh" && \
   grep -q 'AFTER staple' "$ROOT/scripts/release/notarize-macos-app.sh"; then
    pass "notarize-macos-app.sh generates SHA256 after staple"
else
    fail "notarize-macos-app.sh does not generate SHA256 after staple"
fi

# Verify SHA256 generation comes AFTER the actual staple command (not a comment)
STAPLE_LINE="$(
    grep -nE '^[[:space:]]*xcrun[[:space:]]+stapler[[:space:]]+staple[[:space:]]' \
        "$ROOT/scripts/release/notarize-macos-app.sh" |
    head -1 |
    cut -d: -f1
)"
SHA256_LINE="$(
    grep -nE '^[[:space:]]*SHA256=.*shasum[[:space:]]+-a[[:space:]]+256' \
        "$ROOT/scripts/release/notarize-macos-app.sh" |
    head -1 |
    cut -d: -f1
)"
if [[ -z "$STAPLE_LINE" || -z "$SHA256_LINE" ]]; then
    fail "actual staple or SHA256 command not found (staple=$STAPLE_LINE sha256=$SHA256_LINE)"
elif (( SHA256_LINE > STAPLE_LINE )); then
    pass "final SHA256 is generated after actual staple command (staple=$STAPLE_LINE sha256=$SHA256_LINE)"
else
    fail "SHA256 is generated before or at staple (staple=$STAPLE_LINE sha256=$SHA256_LINE)"
fi

echo ""

# ---------------------------------------------------------------------------
# Test 7: Entitlements file contains only minimal permissions
# ---------------------------------------------------------------------------
echo "--- Test 7: Entitlements minimal permissions ---"

ENTITLEMENTS="$ROOT/cmd/ndg-desktop/build/darwin/entitlements.plist"

# Must have allow-jit
if grep -q 'com.apple.security.cs.allow-jit' "$ENTITLEMENTS"; then
    pass "allow-jit entitlement present"
else
    fail "allow-jit entitlement missing"
fi

# Must have allow-unsigned-executable-memory
if grep -q 'com.apple.security.cs.allow-unsigned-executable-memory' "$ENTITLEMENTS"; then
    pass "allow-unsigned-executable-memory entitlement present"
else
    fail "allow-unsigned-executable-memory entitlement missing"
fi

# Must NOT have disable-library-validation
if grep -q 'com.apple.security.cs.disable-library-validation' "$ENTITLEMENTS"; then
    fail "disable-library-validation should not be present (over-broad)"
else
    pass "disable-library-validation correctly absent"
fi

# Must NOT have allow-dyld-environment-variables
if grep -q 'com.apple.security.cs.allow-dyld-environment-variables' "$ENTITLEMENTS"; then
    fail "allow-dyld-environment-variables should not be present (security risk)"
else
    pass "allow-dyld-environment-variables correctly absent"
fi

# Must NOT have network.client/server (sandbox-only, not needed without sandbox)
if grep -q 'com.apple.security.network' "$ENTITLEMENTS"; then
    fail "network entitlement present but not needed (no App Sandbox)"
else
    pass "network entitlements correctly absent (no App Sandbox)"
fi

# Must NOT have files.user-selected (sandbox-only)
if grep -q 'com.apple.security.files' "$ENTITLEMENTS"; then
    fail "files entitlement present but not needed (no App Sandbox)"
else
    pass "files entitlements correctly absent (no App Sandbox)"
fi

# Validate plist syntax using Python plistlib (cross-platform, works on Ubuntu CI)
# plutil is macOS-only and unavailable on Linux runners.
if python3 - "$ENTITLEMENTS" <<'PY'
import plistlib
import sys
with open(sys.argv[1], "rb") as f:
    plistlib.load(f)
PY
then
    pass "entitlements plist is valid (python3 plistlib)"
else
    fail "entitlements plist is invalid XML"
fi

echo ""

# ---------------------------------------------------------------------------
# Test 8: Makefile desktop-release depends on version-check
# ---------------------------------------------------------------------------
echo "--- Test 8: desktop-release depends on version-check ---"

if grep -E '^desktop-release:.*version-check' "$ROOT/Makefile" >/dev/null 2>&1; then
    pass "desktop-release target depends on version-check"
else
    fail "desktop-release target does not depend on version-check"
fi

echo ""

# ---------------------------------------------------------------------------
# Test 9: Makefile desktop-verify supports APP_ONLY=true
# ---------------------------------------------------------------------------
echo "--- Test 9: desktop-verify supports APP_ONLY=true ---"

if grep -A3 '^desktop-verify:' "$ROOT/Makefile" | grep -q 'APP_ONLY'; then
    pass "desktop-verify target supports APP_ONLY=true"
else
    fail "desktop-verify target does not support APP_ONLY"
fi

# Verify --app-only is NOT passed directly in make command
if grep 'desktop-verify.*--app-only' "$ROOT/Makefile" >/dev/null 2>&1; then
    # It's fine if it's inside the if block, just not as a make argument
    if grep -E 'make.*desktop-verify.*--app-only' "$ROOT/Makefile" >/dev/null 2>&1; then
        fail "Makefile passes --app-only directly to make (invalid)"
    else
        pass "desktop-verify uses --app-only only inside script invocation"
    fi
else
    pass "desktop-verify does not pass --app-only as a make argument"
fi

echo ""

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo "========================================"
echo "  test-macos-scripts Summary"
echo "========================================"
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo ""

if [[ $FAIL -gt 0 ]]; then
    echo "test-macos-scripts: FAIL — $FAIL test(s) failed" >&2
    exit 1
fi

echo "test-macos-scripts: OK — all tests passed"
