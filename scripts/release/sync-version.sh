#!/usr/bin/env bash
# sync-version.sh — Synchronize the semantic version from VERSION to all
# build manifests (wails.json, package.json, package-lock.json) and macOS
# Bundle metadata (Info.plist, Info.dev.plist).
#
# The VERSION file is the single source of truth. This script does NOT
# create Git tags; it only updates files to match VERSION.
#
# Usage:
#   ./scripts/release/sync-version.sh
#
# Apple Bundle version mapping:
#   0.5.0-beta.1 → CFBundleShortVersionString=0.5.0, CFBundleVersion=1
#   0.5.0-beta.2 → CFBundleShortVersionString=0.5.0, CFBundleVersion=2
#   0.5.0        → CFBundleShortVersionString=0.5.0, CFBundleVersion=3
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION_FILE="$ROOT/VERSION"

if [[ ! -f "$VERSION_FILE" ]]; then
    echo "sync-version: VERSION file not found at $VERSION_FILE" >&2
    exit 1
fi

VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"

# Validate SemVer: MAJOR.MINOR.PATCH with optional -alpha.N, -beta.N, -rc.N
if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$'; then
    echo "sync-version: invalid SemVer '$VERSION'" >&2
    echo "  expected: MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-{alpha|beta|rc}.N" >&2
    exit 1
fi

echo "sync-version: VERSION = $VERSION"

# --- Derive macOS Bundle version components ---

# CFBundleShortVersionString: strip pre-release suffix (0.5.0-beta.1 → 0.5.0)
SHORT_VERSION="${VERSION%%-*}"

# CFBundleVersion: extract the numeric build number from pre-release suffix.
# -beta.1 → 1, -beta.2 → 2, -rc.1 → 1
# For stable releases (no suffix), use a monotonically increasing number.
# Since we cannot know the previous build number here, stable releases
# require manual CFBundleVersion increment. We use the pre-release number
# if present, or fall back to 1 for stable.
BUILD_NUMBER="1"
if [[ "$VERSION" == *-* ]]; then
    BUILD_NUMBER="${VERSION##*.}"
fi

echo "sync-version: CFBundleShortVersionString = $SHORT_VERSION"
echo "sync-version: CFBundleVersion = $BUILD_NUMBER"

# --- Update wails.json ---
WAILS_JSON="$ROOT/cmd/ndg-desktop/wails.json"
if [[ -f "$WAILS_JSON" ]]; then
    # Use sed to update productVersion in wails.json
    sed -i.bak \
        "s/\"productVersion\": \"[^\"]*\"/\"productVersion\": \"$VERSION\"/" \
        "$WAILS_JSON"
    rm -f "$WAILS_JSON.bak"
    echo "sync-version: updated $WAILS_JSON"
else
    echo "sync-version: WARNING — $WAILS_JSON not found, skipping" >&2
fi

# --- Update package.json ---
PKG_JSON="$ROOT/cmd/ndg-desktop/frontend/package.json"
if [[ -f "$PKG_JSON" ]]; then
    sed -i.bak \
        "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" \
        "$PKG_JSON"
    rm -f "$PKG_JSON.bak"
    echo "sync-version: updated $PKG_JSON"
else
    echo "sync-version: WARNING — $PKG_JSON not found, skipping" >&2
fi

# --- Update package-lock.json ---
LOCK_JSON="$ROOT/cmd/ndg-desktop/frontend/package-lock.json"
if [[ -f "$LOCK_JSON" ]]; then
    # Update both the root "version" and packages[""].version fields.
    # The root version is the 3rd line typically; packages[""] is deeper.
    # We use a targeted sed that matches the exact pattern.
    sed -i.bak \
        -e "3s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" \
        -e "/\"name\": \"ndg-desktop-frontend\"/{n;s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/}" \
        "$LOCK_JSON"
    rm -f "$LOCK_JSON.bak"
    echo "sync-version: updated $LOCK_JSON"
else
    echo "sync-version: WARNING — $LOCK_JSON not found, skipping" >&2
fi

# --- Update Info.plist and Info.dev.plist ---
update_plist() {
    local plist="$1"
    if [[ ! -f "$plist" ]]; then
        echo "sync-version: WARNING — $plist not found, skipping" >&2
        return
    fi
    # CFBundleShortVersionString
    sed -i.bak \
        -e "/<key>CFBundleShortVersionString<\/key>/{n;s/<string>[^<]*<\/string>/<string>$SHORT_VERSION<\/string>/}" \
        -e "/<key>CFBundleVersion<\/key>/{n;s/<string>[^<]*<\/string>/<string>$BUILD_NUMBER<\/string>/}" \
        "$plist"
    rm -f "$plist.bak"
    echo "sync-version: updated $plist"
}

update_plist "$ROOT/cmd/ndg-desktop/build/darwin/Info.plist"
update_plist "$ROOT/cmd/ndg-desktop/build/darwin/Info.dev.plist"

echo "sync-version: done"
