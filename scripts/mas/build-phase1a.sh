#!/usr/bin/env bash
# Local, ad hoc sandbox validation only. Separate output/plist from DMG.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
make wails-check
REVISION="$(git rev-parse HEAD)"
STAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cd cmd/ndg-desktop/frontend
npm ci --ignore-scripts
npm run build
cd "$ROOT"
mkdir -p cmd/ndg-mas-phase1a/frontend/dist
# Use the canonical product icon, never the Wails placeholder fallback.
ICON_SOURCE="${NDG_MAS_ICON_SOURCE:-$ROOT/cmd/ndg-desktop/build/appicon.png}"
cp "$ICON_SOURCE" cmd/ndg-desktop/build/mas-phase1a/appicon.png
cp cmd/ndg-desktop/main.go cmd/ndg-desktop/access_mas_darwin.go cmd/ndg-desktop/access_mas_other.go cmd/ndg-mas-phase1a/
cp -R cmd/ndg-desktop/frontend/dist/. cmd/ndg-mas-phase1a/frontend/dist/
python3 - <<'PYCFG'
import json
from pathlib import Path
entry=Path('cmd/ndg-mas-phase1a/main.go');entry.write_text('//go:build mas\n\n'+entry.read_text())
c=json.loads(Path('cmd/ndg-desktop/wails.mas-phase1a.json').read_text());c['build:dir']='../ndg-desktop/build/mas-phase1a'
Path('cmd/ndg-mas-phase1a/wails.json').write_text(json.dumps(c))
PYCFG
cd cmd/ndg-mas-phase1a
wails build -s -m -nosyncgomod -skipbindings -tags mas -platform darwin/arm64 -ldflags "-X github.com/FNB2026/nas-data-governance/internal/version.Version=dev -X github.com/FNB2026/nas-data-governance/internal/version.Channel=dev -X github.com/FNB2026/nas-data-governance/internal/version.Commit=$REVISION -X github.com/FNB2026/nas-data-governance/internal/version.BuildTime=$STAMP"
cd "$ROOT/cmd/ndg-desktop"
codesign --force --sign - --entitlements build/mas-phase1a/darwin/entitlements.plist 'build/mas-phase1a/bin/NDG MAS Phase1A.app'
codesign --verify --strict 'build/mas-phase1a/bin/NDG MAS Phase1A.app'
