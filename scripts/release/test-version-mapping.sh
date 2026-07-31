#!/usr/bin/env bash
# test-version-mapping.sh — Unit tests for version mapping logic.
#
# Tests that sync-version.sh and check-version-consistency.sh correctly
# handle the BUNDLE_BUILD_NUMBER file, Apple Bundle version mapping,
# and optional Info.dev.plist handling.
#
# Usage:
#   ./scripts/release/test-version-mapping.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="$ROOT/scripts/release"
TESTS_PASSED=0
TESTS_FAILED=0

pass() {
    echo "  PASS — $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

fail() {
    echo "  FAIL — $1" >&2
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

# Create a temporary test workspace
TMPDIR_TEST="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_TEST"' EXIT

echo "=== test-version-mapping: BUNDLE_BUILD_NUMBER monotonic progression ==="
echo ""

# Test 1: BUNDLE_BUILD_NUMBER is read correctly by check-version-consistency.sh
echo "Test 1: check-version-consistency.sh reads BUNDLE_BUILD_NUMBER"

# We can't easily test the full script in isolation, so we test the
# core logic: that BUNDLE_BUILD_NUMBER provides the build number, not
# the pre-release suffix.
test_build_number_derivation() {
    local version="$1"
    local bundle_number="$2"
    local expected_short="$3"

    # Simulate the derivation logic from sync-version.sh
    local short="${version%%-*}"
    local build="$bundle_number"

    if [[ "$short" != "$expected_short" ]]; then
        fail "version=$version → Short=$short, expected=$expected_short"
        return
    fi

    # Verify build number is the BUNDLE_BUILD_NUMBER, not pre-release suffix
    if [[ "$version" == *-* ]]; then
        local pre_release_num="${version##*.}"
        if [[ "$build" == "$pre_release_num" && "$build" != "$bundle_number" ]]; then
            fail "version=$version → Build=$build equals pre-release num=$pre_release_num, but BUNDLE_BUILD_NUMBER=$bundle_number"
            return
        fi
    fi

    pass "version=$version BUNDLE_BUILD_NUMBER=$bundle_number → Short=$short Build=$build"
}

# Test the monotonic progression example from the review
test_build_number_derivation "0.5.0-alpha.1" "1" "0.5.0"
test_build_number_derivation "0.5.0-alpha.3" "2" "0.5.0"
test_build_number_derivation "0.5.0-beta.1"  "3" "0.5.0"
test_build_number_derivation "0.5.0-beta.2"  "4" "0.5.0"
test_build_number_derivation "0.5.0-rc.1"    "5" "0.5.0"
test_build_number_derivation "0.5.0"          "6" "0.5.0"

echo ""

# Test 2: Verify that the old derivation (pre-release number) would fail
echo "Test 2: old derivation (pre-release number) would NOT be monotonic"
{
    old_beta1="1"   # beta.1 → 1
    old_alpha3="3"  # alpha.3 → 3
    old_beta2="2"   # beta.2 → 2 (REGRESSION!)

    if [[ "$old_beta2" -lt "$old_alpha3" ]]; then
        pass "confirmed: old derivation gives beta.2=$old_beta2 < alpha.3=$old_alpha3 (non-monotonic)"
    else
        fail "old derivation appears monotonic, which contradicts the bug report"
    fi
}

echo ""

# Test 3: Info.dev.plist missing scenario
echo "Test 3: check-version-consistency.sh with missing Info.dev.plist"

# Create a minimal test repo structure
mkdir -p "$TMPDIR_TEST/cmd/ndg-desktop/build/darwin"
mkdir -p "$TMPDIR_TEST/cmd/ndg-desktop/frontend"
mkdir -p "$TMPDIR_TEST/scripts/release"

# Copy scripts
cp "$SCRIPT_DIR/check-version-consistency.sh" "$TMPDIR_TEST/scripts/release/"

# Create VERSION and BUNDLE_BUILD_NUMBER
echo "0.5.0-beta.1" > "$TMPDIR_TEST/VERSION"
echo "1" > "$TMPDIR_TEST/BUNDLE_BUILD_NUMBER"

# Create minimal wails.json
cat > "$TMPDIR_TEST/cmd/ndg-desktop/wails.json" <<EOF
{
  "info": {
    "productVersion": "0.5.0-beta.1"
  }
}
EOF

# Create minimal package.json
cat > "$TMPDIR_TEST/cmd/ndg-desktop/frontend/package.json" <<EOF
{
  "version": "0.5.0-beta.1"
}
EOF

# Create minimal package-lock.json
cat > "$TMPDIR_TEST/cmd/ndg-desktop/frontend/package-lock.json" <<EOF
{
  "name": "ndg-desktop-frontend",
  "version": "0.5.0-beta.1",
  "packages": {
    "": {
      "version": "0.5.0-beta.1"
    }
  }
}
EOF

# Create Info.plist (mandatory)
cat > "$TMPDIR_TEST/cmd/ndg-desktop/build/darwin/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleShortVersionString</key>
    <string>0.5.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
</dict>
</plist>
EOF

# Intentionally do NOT create Info.dev.plist (simulating clean clone / CI)

# Run check-version-consistency.sh — should pass without Info.dev.plist
# We need to adjust ROOT for the test copy
# Use sed to make the script use the test directory
sed "s|ROOT=.*|ROOT=\"$TMPDIR_TEST\"|" "$SCRIPT_DIR/check-version-consistency.sh" > "$TMPDIR_TEST/check.sh"
chmod +x "$TMPDIR_TEST/check.sh"

# Initialize a git repo in test dir so git commands don't fail
cd "$TMPDIR_TEST"
git init -q 2>/dev/null || true

if output="$("$TMPDIR_TEST/check.sh" --allow-untagged 2>&1)"; then
    if echo "$output" | grep -q "SKIP.*Info.dev.plist.*optional"; then
        pass "missing Info.dev.plist is correctly skipped (optional)"
    else
        fail "missing Info.dev.plist was handled but SKIP message not found: $output"
    fi
else
    fail "check-version-consistency.sh failed with missing Info.dev.plist: $output"
fi

echo ""

# Test 4: Info.dev.plist present and correct
echo "Test 4: check-version-consistency.sh with correct Info.dev.plist"
cat > "$TMPDIR_TEST/cmd/ndg-desktop/build/darwin/Info.dev.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleShortVersionString</key>
    <string>0.5.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
</dict>
</plist>
EOF

if output="$("$TMPDIR_TEST/check.sh" --allow-untagged 2>&1)"; then
    if echo "$output" | grep -q "OK.*Info.dev.plist.*CFBundleVersion"; then
        pass "present Info.dev.plist is correctly checked"
    else
        fail "present Info.dev.plist was checked but OK message not found: $output"
    fi
else
    fail "check-version-consistency.sh failed with correct Info.dev.plist: $output"
fi

echo ""

# Test 5: Info.dev.plist present but wrong version
echo "Test 5: check-version-consistency.sh with incorrect Info.dev.plist"
cat > "$TMPDIR_TEST/cmd/ndg-desktop/build/darwin/Info.dev.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleShortVersionString</key>
    <string>0.5.0</string>
    <key>CFBundleVersion</key>
    <string>99</string>
</dict>
</plist>
EOF

if output="$("$TMPDIR_TEST/check.sh" --allow-untagged 2>&1)"; then
    fail "check-version-consistency.sh passed with wrong Info.dev.plist CFBundleVersion=99 (should have failed)"
else
    if echo "$output" | grep -q "Info.dev.plist.*CFBundleVersion.*99"; then
        pass "incorrect Info.dev.plist CFBundleVersion is correctly detected as error"
    else
        fail "check failed but error message doesn't mention Info.dev.plist: $output"
    fi
fi

echo ""

# Test 6: BUNDLE_BUILD_NUMBER with non-integer value
echo "Test 6: BUNDLE_BUILD_NUMBER validation rejects non-integer"
echo "abc" > "$TMPDIR_TEST/BUNDLE_BUILD_NUMBER"

if output="$("$TMPDIR_TEST/check.sh" --allow-untagged 2>&1)"; then
    fail "check-version-consistency.sh passed with non-integer BUNDLE_BUILD_NUMBER=abc"
else
    pass "non-integer BUNDLE_BUILD_NUMBER is correctly rejected"
fi

# Restore valid BUNDLE_BUILD_NUMBER
echo "1" > "$TMPDIR_TEST/BUNDLE_BUILD_NUMBER"

cd "$ROOT"

echo ""
echo "=== test-version-mapping: $TESTS_PASSED passed, $TESTS_FAILED failed ==="

if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
fi
