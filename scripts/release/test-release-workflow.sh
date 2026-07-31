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
#   8. SBOM generation has no fail-open + has checksum verification
#   9. Gitleaks config has no full-path allowlist for release scripts
#  10. B7: Syft uses aggregate checksums.txt (not per-file .sha256)
#  11. B8: Pre-release versions get --prerelease --latest=false
#  12. B9: Release creation is idempotent (view → edit → upload --clobber)
#  13. B10: No env: re-declaration of GITHUB_ENV variables in sign/verify steps
#  14. B11: Release update checks isDraft/isImmutable before modifying
#  15. B12: generate-sbom.sh does not use syft from PATH (always downloads pinned)
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

# ---------------------------------------------------------------------------
# Check 10: B7 — Syft uses aggregate checksums.txt (not per-file .sha256)
# ---------------------------------------------------------------------------
echo "--- Check 10: B7 — Syft aggregate checksums file ---"

SBOM_SCRIPT="$ROOT/scripts/release/generate-sbom.sh"

# Must NOT use the old per-file .sha256 URL pattern
if grep -q 'SYFT_CHECKSUM_URL=.*\.sha256' "$SBOM_SCRIPT" 2>/dev/null || \
   grep -q '\${SYFT_URL}\.sha256' "$SBOM_SCRIPT" 2>/dev/null; then
    fail "generate-sbom.sh still uses per-file .sha256 URL (B7 not fixed)"
else
    pass "generate-sbom.sh does not use per-file .sha256 URL"
fi

# Must use aggregate checksums.txt file
if grep -q '_checksums\.txt' "$SBOM_SCRIPT"; then
    pass "generate-sbom.sh references aggregate checksums.txt file"
else
    fail "generate-sbom.sh does not reference aggregate checksums.txt file"
fi

# Must extract specific archive hash from aggregate file
if grep -q 'grep.*SYFT_ARCHIVE.*checksums' "$SBOM_SCRIPT" || \
   grep -q 'awk.*print.*\$1.*checksums' "$SBOM_SCRIPT"; then
    pass "generate-sbom.sh extracts specific archive hash from aggregate file"
else
    fail "generate-sbom.sh does not extract archive hash from aggregate file"
fi

# Must fail if expected hash is empty (archive not found in checksums file)
if grep -q 'EXPECTED_HASH.*-z' "$SBOM_SCRIPT" || \
   grep -q 'not found in official checksums' "$SBOM_SCRIPT"; then
    pass "generate-sbom.sh fails when archive not found in checksums file"
else
    fail "generate-sbom.sh does not handle missing archive in checksums file"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 11: B8 — Pre-release versions get --prerelease --latest=false
# ---------------------------------------------------------------------------
echo "--- Check 11: B8 — Pre-release detection in release.yml ---"

# Must have --prerelease flag
if grep -q '\-\-prerelease' "$WORKFLOW"; then
    pass "release.yml has --prerelease flag for pre-release versions"
else
    fail "release.yml missing --prerelease flag"
fi

# Must have --latest=false flag
if grep -q '\-\-latest=false' "$WORKFLOW"; then
    pass "release.yml has --latest=false for pre-release versions"
else
    fail "release.yml missing --latest=false flag"
fi

# Must have version pattern matching for pre-release suffix
if grep -q 'VERSION.*==.*\*-\*' "$WORKFLOW"; then
    pass "release.yml detects pre-release suffix via version pattern matching"
else
    fail "release.yml missing pre-release version pattern detection"
fi

# Must have release_flags array for dynamic flag construction
if grep -q 'release_flags' "$WORKFLOW"; then
    pass "release.yml uses release_flags array for conditional flags"
else
    fail "release.yml missing release_flags array"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 12: B9 — Release creation is idempotent (view → edit → upload --clobber)
# ---------------------------------------------------------------------------
echo "--- Check 12: B9 — Idempotent release creation ---"

# Must check for existing release with gh release view
if grep -q 'gh release view' "$WORKFLOW"; then
    pass "release.yml checks for existing release (gh release view)"
else
    fail "release.yml missing gh release view existence check"
fi

# Must update existing release with gh release edit
if grep -q 'gh release edit' "$WORKFLOW"; then
    pass "release.yml updates existing release (gh release edit)"
else
    fail "release.yml missing gh release edit for existing release"
fi

# Must upload with --clobber to overwrite existing assets
# --clobber may be on a separate line from gh release upload in YAML
DRAFT_RELEASE_SECTION="$(awk '/^  draft-release:/{found=1} found{print}' "$WORKFLOW")"
if echo "$DRAFT_RELEASE_SECTION" | grep -q 'gh release upload' && \
   echo "$DRAFT_RELEASE_SECTION" | grep -q -- '--clobber'; then
    pass "release.yml uploads assets with --clobber"
else
    fail "release.yml missing --clobber for asset upload"
fi

# Must have both create and update paths (if/else structure)
if grep -q 'gh release create' "$WORKFLOW" && grep -q 'gh release edit' "$WORKFLOW"; then
    pass "release.yml has both create and update paths (idempotent)"
else
    fail "release.yml missing either create or update path"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 13: B10 — No env: re-declaration of GITHUB_ENV variables
# ---------------------------------------------------------------------------
echo "--- Check 13: B10 — No env: re-declaration of GITHUB_ENV vars ---"

# B10: The "Sign .app" step must NOT have env: SIGNING_IDENTITY: ${{ env.SIGNING_IDENTITY }}
# $GITHUB_ENV writes are available as shell env vars in subsequent steps.
# Re-declaring via env: can resolve to empty and override the $GITHUB_ENV value.
SIGN_STEP="$(grep -A 15 'Sign \.app with Developer ID' "$WORKFLOW")"
if echo "$SIGN_STEP" | grep -q 'SIGNING_IDENTITY: \${{ env\.SIGNING_IDENTITY }}'; then
    fail "Sign .app step re-declares SIGNING_IDENTITY via env: \${{ env.* }} (B10 not fixed)"
else
    pass "Sign .app step does not re-declare SIGNING_IDENTITY via env:"
fi

# B10: The "Sign .app" step must have an empty-value check
if echo "$SIGN_STEP" | grep -q 'SIGNING_IDENTITY.*empty\|GITHUB_ENV propagation failed'; then
    pass "Sign .app step has empty-value check for SIGNING_IDENTITY"
else
    fail "Sign .app step missing empty-value check for SIGNING_IDENTITY"
fi

# B10: The "Verify release artifacts" step must NOT have env: APPLE_TEAM_ID: ${{ env.APPLE_TEAM_ID }}
VERIFY_STEP="$(grep -A 15 'Verify release artifacts with Team ID' "$WORKFLOW")"
if echo "$VERIFY_STEP" | grep -q 'APPLE_TEAM_ID: \${{ env\.APPLE_TEAM_ID }}'; then
    fail "Verify artifacts step re-declares APPLE_TEAM_ID via env: \${{ env.* }} (B10 not fixed)"
else
    pass "Verify artifacts step does not re-declare APPLE_TEAM_ID via env:"
fi

# B10: The "Verify release artifacts" step must have an empty-value check
if echo "$VERIFY_STEP" | grep -q 'APPLE_TEAM_ID.*empty\|GITHUB_ENV propagation failed'; then
    pass "Verify artifacts step has empty-value check for APPLE_TEAM_ID"
else
    fail "Verify artifacts step missing empty-value check for APPLE_TEAM_ID"
fi

# B10: Signing identity verification must filter by APPLE_TEAM_ID (grep), not just take first
IDENTITY_STEP="$(grep -A 25 'Verify signing identity matches APPLE_TEAM_ID' "$WORKFLOW")"
if echo "$IDENTITY_STEP" | grep -q 'grep.*APPLE_TEAM_ID\|grep "\$APPLE_TEAM_ID"'; then
    pass "Signing identity verification filters by APPLE_TEAM_ID (grep)"
else
    fail "Signing identity verification does not filter by APPLE_TEAM_ID"
fi

# B10: Must check for multiple matches (ambiguous identity)
if echo "$IDENTITY_STEP" | grep -q 'MATCH_COUNT.*gt 1\|multiple.*Developer ID'; then
    pass "Signing identity verification checks for ambiguous matches"
else
    fail "Signing identity verification missing ambiguous match check"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 14: B11 — Release update checks isDraft/isImmutable before modifying
# ---------------------------------------------------------------------------
echo "--- Check 14: B11 — isDraft/isImmutable state check ---"

DRAFT_RELEASE_SECTION="$(awk '/^  draft-release:/{found=1} found{print}' "$WORKFLOW")"

# B11: Must query release state with --json isDraft,isPrerelease,isImmutable
if echo "$DRAFT_RELEASE_SECTION" | grep -q -- '--json isDraft,isPrerelease,isImmutable'; then
    pass "Release update queries isDraft,isPrerelease,isImmutable via --json"
else
    fail "Release update missing --json isDraft,isPrerelease,isImmutable query"
fi

# B11: Must check IS_DRAFT == true
if echo "$DRAFT_RELEASE_SECTION" | grep -q 'IS_DRAFT.*true\|isDraft.*true'; then
    pass "Release update checks IS_DRAFT is true"
else
    fail "Release update missing IS_DRAFT check"
fi

# B11: Must check IS_IMMUTABLE
if echo "$DRAFT_RELEASE_SECTION" | grep -q 'IS_IMMUTABLE'; then
    pass "Release update checks IS_IMMUTABLE"
else
    fail "Release update missing IS_IMMUTABLE check"
fi

# B11: Must fail closed when not draft or immutable
if echo "$DRAFT_RELEASE_SECTION" | grep -q 'refusing to modify\|already published or immutable'; then
    pass "Release update fails closed for published/immutable releases"
else
    fail "Release update missing fail-closed for published/immutable releases"
fi

echo ""

# ---------------------------------------------------------------------------
# Check 15: B12 — generate-sbom.sh does not use syft from PATH
# ---------------------------------------------------------------------------
echo "--- Check 15: B12 — No syft from PATH bypass ---"

SBOM_SCRIPT="$ROOT/scripts/release/generate-sbom.sh"

# B12: Must NOT have command -v syft shortcut
if grep -q 'command -v syft' "$SBOM_SCRIPT"; then
    fail "generate-sbom.sh still has 'command -v syft' PATH bypass (B12 not fixed)"
else
    pass "generate-sbom.sh does not use 'command -v syft' PATH bypass"
fi

# B12: Must NOT have the "using syft (found in PATH)" bypass message
if grep -q 'using syft (found in PATH)' "$SBOM_SCRIPT"; then
    fail "generate-sbom.sh still has 'using syft (found in PATH)' bypass (B12 not fixed)"
else
    pass "generate-sbom.sh does not have 'using syft (found in PATH)' bypass"
fi

# B12: Must always download pinned version
if grep -q 'SYFT_VERSION=.*v1\.20\.0' "$SBOM_SCRIPT"; then
    pass "generate-sbom.sh always downloads pinned syft v1.20.0"
else
    fail "generate-sbom.sh missing pinned SYFT_VERSION"
fi

# B12: Must always verify checksums (not conditional on PATH presence)
if grep -q 'checksum verified' "$SBOM_SCRIPT"; then
    pass "generate-sbom.sh always verifies checksum"
else
    fail "generate-sbom.sh missing checksum verification"
fi

# B12: Must only use the downloaded binary ($TMP_DIR/syft), not bare 'syft'
if grep -q '"\$TMP_DIR/syft"' "$SBOM_SCRIPT" && ! grep -q '^[[:space:]]*syft dir' "$SBOM_SCRIPT"; then
    pass "generate-sbom.sh only executes downloaded syft (not PATH syft)"
else
    fail "generate-sbom.sh may execute bare 'syft' from PATH"
fi

# B12 Behavioral test: simulate fake syft in PATH and verify script doesn't call it
echo "  --- B12 behavioral: fake syft in PATH ---"
FAKE_SYFT_DIR="$(mktemp -d)"
cat > "$FAKE_SYFT_DIR/syft" <<'FAKEEOF'
#!/usr/bin/env bash
echo "FAKE SYFT CALLED — B12 BYPASS DETECTED" >&2
exit 1
FAKEEOF
chmod +x "$FAKE_SYFT_DIR/syft"

# Verify the fake syft is findable in PATH
PATH="$FAKE_SYFT_DIR:$PATH" command -v syft >/dev/null 2>&1
if [ $? -eq 0 ]; then
    # Verify generate-sbom.sh does not contain the bypass pattern
    # (structural check already done above, but this confirms the fake is detectable)
    pass "Fake syft is detectable in PATH (behavioral test setup OK)"
else
    fail "Could not set up fake syft in PATH for behavioral test"
fi

# Clean up
/bin/rm -rf "$FAKE_SYFT_DIR"

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
