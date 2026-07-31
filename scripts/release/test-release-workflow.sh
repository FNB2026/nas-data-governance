#!/usr/bin/env bash
# test-release-workflow.sh — Validate release.yml workflow security requirements.
#
# B6: The release workflow only runs on tag push, so CI cannot exercise it
# directly. This script performs structural validation to ensure the workflow
# meets security requirements before a real tag release is attempted.
#
# Checks:
#   1. Global permissions are read-only
#   2. Only draft-release job has contents: write
#   3. All `uses:` actions are pinned to full 40-char commit SHA
#   4. .app is uploaded as tarball (not raw directory)
#   5. sign-notarize job uses release-macos environment
#   6. Four-job needs chain is intact (verify → build-unsigned → sign-notarize → draft-release)
#   7. Global permissions does not contain contents: write
#
# Usage:
#   ./scripts/release/test-release-workflow.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORKFLOW="$ROOT/.github/workflows/release.yml"
CI_WORKFLOW="$ROOT/.github/workflows/ci.yml"

PASS=0
FAIL=0

pass() { echo "  PASS — $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL — $1" >&2; FAIL=$((FAIL + 1)); }

echo "=== test-release-workflow: Release workflow security validation ==="
echo ""

# ---------------------------------------------------------------------------
# Check 1: Global permissions are read-only
# ---------------------------------------------------------------------------
echo "--- Check 1: Global permissions are read-only ---"

# Extract the first top-level permissions block (not indented)
GLOBAL_PERMS="$(awk '/^permissions:/{found=1} found{print} found && /^$/{exit}' "$WORKFLOW")"

if echo "$GLOBAL_PERMS" | grep -q 'contents: read'; then
    pass "Global permissions set to contents: read"
else
    fail "Global permissions not set to contents: read"
fi

if echo "$GLOBAL_PERMS" | grep -q 'contents: write'; then
    fail "Global permissions contain contents: write (should be read-only)"
else
    pass "Global permissions do not contain contents: write"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 2: Only draft-release job has contents: write
# ---------------------------------------------------------------------------
echo "--- Check 2: Only draft-release has write permission ---"

# Check draft-release has write permission
# draft-release is the last job, so grep -A context is safe
if grep -A30 '^  draft-release:' "$WORKFLOW" | grep -q 'contents: write'; then
    pass "draft-release job has contents: write permission"
else
    fail "draft-release job missing contents: write permission"
fi

# Check that no other job (verify, build-unsigned, sign-notarize) has contents: write
for job in verify build-unsigned sign-notarize; do
    JOB_PERMS="$(grep -A30 "^  ${job}:" "$WORKFLOW" | sed '/^  [a-z].*:$/q')"
    if echo "$JOB_PERMS" | grep -q 'contents: write'; then
        fail "$job job has contents: write (should not)"
    else
        pass "$job job does not have contents: write"
    fi
done

echo ""

# ---------------------------------------------------------------------------
# Check 3: All `uses:` actions are pinned to full 40-char commit SHA
# ---------------------------------------------------------------------------
echo "--- Check 3: All actions pinned to commit SHA ---"

# Check release.yml
SHA_PATTERN='@[0-9a-f]{40}'
NON_SHA_COUNT=0

while IFS= read -r line; do
    # Extract the action reference after 'uses:'
    action_ref="$(echo "$line" | sed 's/.*uses: *//; s/ *#.*//')"
    if [[ "$action_ref" != *"$SHA_PATTERN"* ]] && ! echo "$action_ref" | grep -qE "$SHA_PATTERN"; then
        fail "release.yml: Action not pinned to SHA: $action_ref"
        NON_SHA_COUNT=$((NON_SHA_COUNT + 1))
    fi
done < <(grep 'uses:' "$WORKFLOW")

if [[ $NON_SHA_COUNT -eq 0 ]]; then
    pass "All release.yml actions are pinned to commit SHA"
fi

# Check ci.yml as well
CI_NON_SHA_COUNT=0
while IFS= read -r line; do
    action_ref="$(echo "$line" | sed 's/.*uses: *//; s/ *#.*//')"
    if [[ "$action_ref" != *"$SHA_PATTERN"* ]] && ! echo "$action_ref" | grep -qE "$SHA_PATTERN"; then
        fail "ci.yml: Action not pinned to SHA: $action_ref"
        CI_NON_SHA_COUNT=$((CI_NON_SHA_COUNT + 1))
    fi
done < <(grep 'uses:' "$CI_WORKFLOW")

if [[ $CI_NON_SHA_COUNT -eq 0 ]]; then
    pass "All ci.yml actions are pinned to commit SHA"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 4: .app is uploaded as tarball (not raw directory)
# ---------------------------------------------------------------------------
echo "--- Check 4: .app uploaded as tarball ---"

if grep -q 'unsigned-app.tar.gz' "$WORKFLOW" && grep -q -- '-czf.*unsigned-app.tar.gz' "$WORKFLOW"; then
    pass ".app is tar'd before upload to preserve permissions"
else
    fail ".app is not tar'd before upload (permission loss risk)"
fi

# Check tar extraction and executable verification exists
if grep -q 'tar.*-xzf.*unsigned-app.tar.gz' "$WORKFLOW"; then
    pass ".app tarball is extracted after download"
else
    fail ".app tarball extraction step not found"
fi

if grep -q 'test -x\|\[\[ ! -x' "$WORKFLOW"; then
    pass "Executable permission is verified after untar"
else
    fail "Executable permission verification not found after untar"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 5: sign-notarize uses release-macos environment
# ---------------------------------------------------------------------------
echo "--- Check 5: sign-notarize uses release-macos environment ---"

if grep -A5 '^  sign-notarize:' "$WORKFLOW" | grep -q 'environment: release-macos'; then
    pass "sign-notarize job uses release-macos environment"
else
    fail "sign-notarize job missing release-macos environment"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 6: Four-job needs chain is intact
# ---------------------------------------------------------------------------
echo "--- Check 6: Job needs chain ---"

# build-unsigned needs verify
if grep -A10 '^  build-unsigned:' "$WORKFLOW" | grep -q 'needs: verify'; then
    pass "build-unsigned needs verify"
else
    fail "build-unsigned does not need verify"
fi

# sign-notarize needs build-unsigned
if grep -A10 '^  sign-notarize:' "$WORKFLOW" | grep -q 'needs: build-unsigned'; then
    pass "sign-notarize needs build-unsigned"
else
    fail "sign-notarize does not need build-unsigned"
fi

# draft-release needs sign-notarize
if grep -A10 '^  draft-release:' "$WORKFLOW" | grep -q 'needs: sign-notarize'; then
    pass "draft-release needs sign-notarize"
else
    fail "draft-release does not need sign-notarize"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 7: APPLE_TEAM_ID is used for signing identity verification
# ---------------------------------------------------------------------------
echo "--- Check 7: APPLE_TEAM_ID signing identity verification ---"

if grep -q 'Verify signing identity matches APPLE_TEAM_ID' "$WORKFLOW"; then
    pass "Signing identity verification step exists"
else
    fail "Signing identity verification step not found"
fi

if grep -q 'expected-team-id' "$WORKFLOW"; then
    pass "Team ID assertion passed to verify script"
else
    fail "Team ID assertion not passed to verify script"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 8: SBOM generation does not use fail-open (|| true)
# ---------------------------------------------------------------------------
echo "--- Check 8: SBOM generation has no fail-open ---"

SBOM_SCRIPT="$ROOT/scripts/release/generate-sbom.sh"
if grep -q '|| true' "$SBOM_SCRIPT"; then
    fail "generate-sbom.sh still contains '|| true' (fail-open)"
else
    pass "generate-sbom.sh has no fail-open (|| true)"
fi

if grep -q 'validate_sbom_output' "$SBOM_SCRIPT"; then
    pass "generate-sbom.sh has JSON validation function"
else
    fail "generate-sbom.sh missing JSON validation function"
fi

if grep -q 'sha256\|SHA256\|checksum' "$SBOM_SCRIPT"; then
    pass "generate-sbom.sh has SHA256 checksum verification for syft download"
else
    fail "generate-sbom.sh missing SHA256 checksum verification for syft download"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 9: Gitleaks config has no full-path allowlist for release scripts
# ---------------------------------------------------------------------------
echo "--- Check 9: Gitleaks config security ---"

GITLEAKS_CONFIG="$ROOT/.gitleaks.toml"
# Check that the global allowlist does not have a paths = [ ... ] section
# that includes scripts/release/*.sh or .github/workflows/release.yml
if grep -A20 '^\[allowlist\]' "$GITLEAKS_CONFIG" | grep -q 'scripts/release/\*\.sh'; then
    fail ".gitleaks.toml global allowlist still bypasses release scripts"
else
    pass ".gitleaks.toml does not globally bypass release scripts"
fi

if grep -A20 '^\[allowlist\]' "$GITLEAKS_CONFIG" | grep -q 'release\.yml'; then
    fail ".gitleaks.toml global allowlist still bypasses release.yml"
else
    pass ".gitleaks.toml does not globally bypass release.yml"
fi

echo ""

# ===========================================================================
# Summary
# ===========================================================================
echo "========================================"
echo "  test-release-workflow Summary"
echo "========================================"
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo ""

if [[ $FAIL -gt 0 ]]; then
    echo "test-release-workflow: FAIL — $FAIL check(s) failed" >&2
    exit 1
fi

echo "test-release-workflow: OK — all checks passed"
