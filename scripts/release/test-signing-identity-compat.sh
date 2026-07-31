#!/usr/bin/env bash
# test-signing-identity-compat.sh — Behavioral test for Bash 3.2-compatible
# signing identity selection logic.
#
# B15: macOS GitHub Runner uses system Bash 3.2 which does not support
# mapfile/readarray. This test verifies that the identity-selection logic
# (extracted from release.yml) works correctly under /bin/bash by feeding
# it simulated `security find-identity` output.
#
# This test does NOT access the real Keychain, does NOT require Apple
# certificates, and does NOT perform any signing. It only tests the
# parsing and selection logic.
#
# Test scenarios:
#   1. Zero matches — fail closed
#   2. Single exact match — success
#   3. Multiple matches — fail closed
#   4. Name contains Team ID string but bracket ID differs — no match
#   5. Invalid APPLE_TEAM_ID formats — all rejected
#   6. Identity name with spaces — preserved correctly
#
# Usage:
#   /bin/bash ./scripts/release/test-signing-identity-compat.sh
set -euo pipefail

PASS=0
FAIL=0

pass() { echo "  PASS — $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL — $1" >&2; FAIL=$((FAIL + 1)); }

# ---------------------------------------------------------------------------
# Core logic under test — mirrors the release.yml identity selection step.
# This function reads simulated `security find-identity -v -p codesigning`
# output from stdin and selects the unique matching Developer ID Application
# certificate.
#
# Arguments:
#   $1 — APPLE_TEAM_ID (10 uppercase alphanumeric characters)
#   stdin — simulated security find-identity output
#
# Output:
#   Sets SIGNING_IDENTITY on success (also prints it to stdout).
#
# Returns:
#   0 — exactly one match found, SIGNING_IDENTITY set
#   1 — zero matches, multiple matches, or invalid Team ID
# ---------------------------------------------------------------------------
select_signing_identity() {
    local apple_team_id="$1"
    local identity
    local matching_count
    local -a matching_identities

    # B13: Validate APPLE_TEAM_ID format (10 alphanumeric chars)
    if [[ ! "$apple_team_id" =~ ^[A-Z0-9]{10}$ ]]; then
        echo "FAIL — invalid APPLE_TEAM_ID format: '$apple_team_id'" >&2
        return 1
    fi

    # B15: Bash 3.2-compatible array collection (no mapfile/readarray).
    matching_identities=()
    while IFS= read -r identity; do
        if [[ -n "$identity" ]]; then
            matching_identities[${#matching_identities[@]}]="$identity"
        fi
    done < <(
        awk -F '"' '/Developer ID Application/ { print $2 }' \
            | grep -F "($apple_team_id)" || true
    )

    matching_count=${#matching_identities[@]}

    case "$matching_count" in
        0)
            echo "FAIL — no matching Developer ID identity" >&2
            return 1
            ;;
        1)
            SIGNING_IDENTITY="${matching_identities[0]}"
            echo "$SIGNING_IDENTITY"
            return 0
            ;;
        *)
            echo "FAIL — multiple ($matching_count) matching identities" >&2
            return 1
            ;;
    esac
}

echo "=== test-signing-identity-compat: Bash 3.2 identity selection ==="
echo ""

# ---------------------------------------------------------------------------
# Scenario 1: Zero matches — no Developer ID Application certificate
# ---------------------------------------------------------------------------
echo "--- Scenario 1: Zero matches ---"
RESULT=""
if RESULT="$(echo '1) HASH "Apple Development: Example (ZZZZZ99999)"' \
    | select_signing_identity "ABCDE12345" 2>/dev/null)"; then
    fail "Zero-match scenario should fail (got: $RESULT)"
else
    pass "Zero-match scenario correctly fails (exit non-zero)"
fi
if [[ -z "${RESULT:-}" ]]; then
    pass "SIGNING_IDENTITY not set on zero match"
else
    fail "SIGNING_IDENTITY should be empty on zero match (got: $RESULT)"
fi
echo ""

# ---------------------------------------------------------------------------
# Scenario 2: Single exact match — success
# ---------------------------------------------------------------------------
echo "--- Scenario 2: Single exact match ---"
INPUT='1) HASH1 "Developer ID Application: Example Company (ABCDE12345)"
1 valid identity found'
EXPECTED="Developer ID Application: Example Company (ABCDE12345)"
RESULT=""
if RESULT="$(echo "$INPUT" | select_signing_identity "ABCDE12345" 2>/dev/null)"; then
    pass "Single-match scenario succeeds (exit 0)"
else
    fail "Single-match scenario should succeed (exit 0)"
fi
if [[ "$RESULT" == "$EXPECTED" ]]; then
    pass "SIGNING_IDENTITY equals full certificate name"
else
    fail "SIGNING_IDENTITY mismatch (got: '$RESULT', expected: '$EXPECTED')"
fi
echo ""

# ---------------------------------------------------------------------------
# Scenario 3: Multiple matches — fail closed
# ---------------------------------------------------------------------------
echo "--- Scenario 3: Multiple matches ---"
INPUT='1) HASH1 "Developer ID Application: Example One (ABCDE12345)"
2) HASH2 "Developer ID Application: Example Two (ABCDE12345)"
2 valid identities found'
RESULT=""
if RESULT="$(echo "$INPUT" | select_signing_identity "ABCDE12345" 2>/dev/null)"; then
    fail "Multiple-match scenario should fail (got: $RESULT)"
else
    pass "Multiple-match scenario correctly fails (exit non-zero)"
fi
if [[ -z "${RESULT:-}" ]]; then
    pass "SIGNING_IDENTITY not set on multiple match"
else
    fail "SIGNING_IDENTITY should be empty on multiple match (got: $RESULT)"
fi
echo ""

# ---------------------------------------------------------------------------
# Scenario 4: Name contains Team ID string but bracket ID differs — no match
# ---------------------------------------------------------------------------
echo "--- Scenario 4: Substring in name, bracket ID differs ---"
INPUT='1) HASH1 "Developer ID Application: ABCDE12345 Example Company (ZZZZZ99999)"
1 valid identity found'
RESULT=""
if RESULT="$(echo "$INPUT" | select_signing_identity "ABCDE12345" 2>/dev/null)"; then
    fail "Substring-name scenario should NOT match (got: $RESULT)"
else
    pass "Substring-name scenario correctly does not match (exit non-zero)"
fi
echo ""

# ---------------------------------------------------------------------------
# Scenario 5: Invalid APPLE_TEAM_ID formats — all rejected
# ---------------------------------------------------------------------------
echo "--- Scenario 5: Invalid APPLE_TEAM_ID formats ---"
INPUT='1) HASH1 "Developer ID Application: Example Company (ABCDE12345)"
1 valid identity found'

for bad_id in "abcde12345" "ABCDE1234" "ABCDE12345X" "ABCDE-2345"; do
    if echo "$INPUT" | select_signing_identity "$bad_id" 2>/dev/null; then
        fail "Invalid Team ID '$bad_id' should be rejected"
    else
        pass "Invalid Team ID '$bad_id' correctly rejected"
    fi
done
echo ""

# ---------------------------------------------------------------------------
# Scenario 6: Identity name with spaces — preserved correctly
# ---------------------------------------------------------------------------
echo "--- Scenario 6: Identity name with spaces ---"
INPUT='1) HASH1 "Developer ID Application: My Company Name LLC (ABCDE12345)"
1 valid identity found'
EXPECTED="Developer ID Application: My Company Name LLC (ABCDE12345)"
RESULT=""
if RESULT="$(echo "$INPUT" | select_signing_identity "ABCDE12345" 2>/dev/null)"; then
    pass "Spaces-in-name scenario succeeds (exit 0)"
else
    fail "Spaces-in-name scenario should succeed"
fi
if [[ "$RESULT" == "$EXPECTED" ]]; then
    pass "Full name with spaces preserved (no word splitting)"
else
    fail "Name with spaces not preserved (got: '$RESULT', expected: '$EXPECTED')"
fi
echo ""

# ===========================================================================
# Summary
# ===========================================================================
echo "========================================"
echo "  test-signing-identity-compat Summary"
echo "========================================"
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "========================================"

if [[ "$FAIL" -gt 0 ]]; then
    echo "test-signing-identity-compat: FAIL — $FAIL test(s) failed" >&2
    exit 1
fi

echo "test-signing-identity-compat: OK — all tests passed"
exit 0
