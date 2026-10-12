#!/usr/bin/env bash
# Local sandbox proof only. Does not sign/upload a store distribution or touch DMG.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
(cd "$ROOT" && make wails-check)
cd "$ROOT/cmd/ndg-desktop/frontend"
npm ci --ignore-scripts
cd "$ROOT"
node --input-type=module - <<'JS'
import {createRequire} from 'node:module';
import fs from 'node:fs';
const require=createRequire(process.cwd()+'/cmd/ndg-desktop/frontend/package.json');
await require('esbuild').build({entryPoints:['cmd/ndg-mas-probe/frontend/src/main.jsx'],bundle:true,minify:true,outfile:'cmd/ndg-mas-probe/frontend/dist/main.js',nodePaths:[process.cwd()+'/cmd/ndg-desktop/frontend/node_modules'],define:{'process.env.NODE_ENV':'"production"'}});
fs.copyFileSync('cmd/ndg-mas-probe/frontend/index.html','cmd/ndg-mas-probe/frontend/dist/index.html');
JS
cd "$ROOT/cmd/ndg-mas-probe"
wails build -s -m -nosyncgomod -skipbindings -tags masprobe -platform darwin/arm64
codesign --force --sign - --entitlements build/darwin/entitlements.plist 'build/bin/NDG MAS Probe.app'
codesign --verify --strict 'build/bin/NDG MAS Probe.app'
