#!/usr/bin/env bash
# sync-version.sh — Synchronize the semantic version from VERSION and the
# bundle build number from BUNDLE_BUILD_NUMBER to all build manifests
# (wails.json, package.json, package-lock.json) and macOS Bundle metadata
# (Info.plist, Info.dev.plist).
#
# VERSION is the single source of truth for SemVer.
# BUNDLE_BUILD_NUMBER is the single source of truth for CFBundleVersion,
# and must be monotonically increasing across all public builds.
#
# This script does NOT create Git tags; it only updates files.
#
# Usage:
#   ./scripts/release/sync-version.sh
#
# Apple Bundle version mapping:
#   VERSION=0.5.0-beta.1  BUNDLE_BUILD_NUMBER=1  → Short=0.5.0, Build=1
#   VERSION=0.5.0-beta.2  BUNDLE_BUILD_NUMBER=2  → Short=0.5.0, Build=2
#   VERSION=0.5.0-rc.1    BUNDLE_BUILD_NUMBER=3  → Short=0.5.0, Build=3
#   VERSION=0.5.0         BUNDLE_BUILD_NUMBER=4  → Short=0.5.0, Build=4
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION_FILE="$ROOT/VERSION"
BUILD_NUMBER_FILE="$ROOT/BUNDLE_BUILD_NUMBER"

if [[ ! -f "$VERSION_FILE" ]]; then
    echo "sync-version: VERSION file not found at $VERSION_FILE" >&2
    exit 1
fi

if [[ ! -f "$BUILD_NUMBER_FILE" ]]; then
    echo "sync-version: BUNDLE_BUILD_NUMBER file not found at $BUILD_NUMBER_FILE" >&2
    exit 1
fi

VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"
BUILD_NUMBER="$(tr -d '[:space:]' < "$BUILD_NUMBER_FILE")"

# Validate SemVer: MAJOR.MINOR.PATCH with optional -alpha.N, -beta.N, -rc.N
if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$'; then
    echo "sync-version: invalid SemVer '$VERSION'" >&2
    echo "  expected: MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-{alpha|beta|rc}.N" >&2
    exit 1
fi

# Validate BUNDLE_BUILD_NUMBER: must be a positive integer
if ! echo "$BUILD_NUMBER" | grep -qE '^[0-9]+$'; then
    echo "sync-version: invalid BUNDLE_BUILD_NUMBER '$BUILD_NUMBER'" >&2
    echo "  expected: a positive integer (e.g. 1, 2, 3, ...)" >&2
    exit 1
fi

echo "sync-version: VERSION = $VERSION"
echo "sync-version: BUNDLE_BUILD_NUMBER = $BUILD_NUMBER"

# --- Derive macOS Bundle version components ---

# CFBundleShortVersionString: strip pre-release suffix (0.5.0-beta.1 → 0.5.0)
SHORT_VERSION="${VERSION%%-*}"

# CFBundleVersion: read from BUNDLE_BUILD_NUMBER (monotonically increasing)
echo "sync-version: CFBundleShortVersionString = $SHORT_VERSION"
echo "sync-version: CFBundleVersion = $BUILD_NUMBER"

# --- Update wails.json ---
WAILS_JSON="$ROOT/cmd/ndg-desktop/wails.json"
if [[ -f "$WAILS_JSON" ]]; then
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
