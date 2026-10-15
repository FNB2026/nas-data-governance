#!/usr/bin/env bash
# Ordinary-desktop acceptance composition, never a release or MAS build.
set -euo pipefail
umask 077
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
MODE="${1:-run}"
case "$MODE" in run|--verify|--build-only) ;; *) echo 'usage: script/build_and_run.sh [--verify|--build-only]' >&2; exit 2;; esac
OUT="$ROOT/var/b2-dev"
APP="$OUT/NDG B2 Dev.app"
BIN="$APP/Contents/MacOS/NDGB2Dev"
# Never kill a release App or another worktree's process. Refuse a live instance
# of this particular executable so its acceptance evidence is preserved.
if pgrep -f "^$BIN" >/dev/null; then
  echo 'B2_DEV_ALREADY_RUNNING: quit this development App before rebuilding' >&2
  exit 1
fi
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
(cd cmd/ndg-desktop/frontend && npm ci --ignore-scripts && npm run build)
REVISION="$(git rev-parse HEAD)"
if ! git diff --quiet HEAD; then REVISION="$REVISION-dirty"; fi
STAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PKG=github.com/FNB2026/nas-data-governance/internal/version
CGO_LDFLAGS="${CGO_LDFLAGS:-} -framework UniformTypeIdentifiers" \
go build -trimpath -tags 'desktop,production,ndg_devprofile' \
  -ldflags "-X $PKG.Version=dev-b2 -X $PKG.Channel=dev -X $PKG.Commit=$REVISION -X $PKG.BuildTime=$STAMP" \
  -o "$BIN" ./cmd/ndg-desktop
cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>NDGB2Dev</string>
<key>CFBundleIdentifier</key><string>com.fnb.ndg.b2.dev</string>
<key>CFBundleName</key><string>NDG B2 Dev</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.0.0</string>
<key>CFBundleVersion</key><string>1</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>NSPrincipalClass</key><string>NSApplication</string>
</dict></plist>
PLIST
codesign --force --sign - "$APP"
codesign --verify --strict "$APP"
shasum -a 256 "$BIN" > "$OUT/binary.sha256"
git diff --binary HEAD > "$OUT/source.patch"
git ls-files --others --exclude-standard -z | python3 -c '
import hashlib, pathlib, sys
h=hashlib.sha256()
for name in sorted(sys.stdin.buffer.read().split(b"\0")):
    if name:
        h.update(name+b"\0"+pathlib.Path(name.decode()).read_bytes())
print(h.hexdigest())' > "$OUT/untracked-source.sha256"
if [[ "$MODE" == --build-only ]]; then exit 0; fi
PROFILE="$(mktemp -d /private/tmp/ndg-b2-XXXXXXXX)"
printf 'ndg-b2-dev-profile-v1\n' > "$PROFILE/.ndg-b2-profile"
printf '%s\n' "$PROFILE" > "$OUT/profile-path"
# The profile override is consumed only by the explicit build tag. HOME is
# unchanged. A different bundle ID is for process/cache identity, not DB isolation.
/usr/bin/open -n "$APP" --env "NDG_DEV_PROFILE=$PROFILE" \
  --stdout "$OUT/stdout.log" --stderr "$OUT/stderr.log"
if [[ "$MODE" == --verify ]]; then
  for _ in {1..20}; do
    if pgrep -f "^$BIN" > "$OUT/pid"; then
      sleep 1
      if kill -0 "$(cat "$OUT/pid")" 2>/dev/null; then echo 'B2_DEV_STARTED'; exit 0; fi
      echo 'B2_DEV_EXITED_DURING_STARTUP' >&2; exit 1
    fi
    sleep 0.25
  done
  echo 'B2_DEV_LAUNCH_FAILED' >&2; exit 1
fi
