#!/usr/bin/env bash
# Build the read-write App Store channel in an isolated output directory.
set -euo pipefail
umask 077
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
mode="${1:-local}"
case "$mode" in
  local) bundle_id=com.fnb.ndg.mas.launchqa ;;
  distribution)
    bundle_id=com.fnb.ndg.mas
    : "${NDG_MAS_PROFILE:?Path to the matching Mac App Store distribution profile}"
    : "${NDG_MAS_APP_IDENTITY:?Apple Distribution or Mac App Distribution identity}"
    : "${NDG_MAS_INSTALLER_IDENTITY:?Mac Installer Distribution identity}"
    : "${NDG_MAS_PRIVACY_MANIFEST:?Path to the completed PrivacyInfo.xcprivacy}"
    case "$NDG_MAS_APP_IDENTITY" in
      "Apple Distribution:"*|"Mac App Distribution:"*|"3rd Party Mac Developer Application:"*) ;;
      *) echo "Expected a Mac App Store application distribution identity" >&2; exit 1 ;;
    esac
    case "$NDG_MAS_INSTALLER_IDENTITY" in
      "Mac Installer Distribution:"*|"3rd Party Mac Developer Installer:"*) ;;
      *) echo "Expected a Mac App Store installer identity" >&2; exit 1 ;;
    esac
    ;;
  *) echo "Usage: $0 [local|distribution]" >&2; exit 2 ;;
esac

output="$(mktemp -d "${TMPDIR:-/tmp}/ndg-mas-launch.XXXXXXXX")"
echo "Output $output"
app="$output/NDG.app"
contents="$app/Contents"
mkdir -p "$contents/MacOS" "$contents/Resources" "$output/NDG.iconset"
version="${NDG_MAS_VERSION:-0.5.0}"
build="${NDG_MAS_BUILD:-1}"
revision="$(git rev-parse HEAD)"
stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
python3 - "$contents/Info.plist" "$bundle_id" "$version" "$build" <<'PY'
import plistlib, re, sys
path, bundle, version, build = sys.argv[1:]
if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version) or not re.fullmatch(r"[1-9][0-9]*", build):
    raise SystemExit("Use a numeric x.y.z app version and a positive integer build number")
with open(path, "wb") as out:
    plistlib.dump(dict(
        CFBundleExecutable="NDG", CFBundleIconFile="NDG.icns", CFBundleIdentifier=bundle,
        CFBundleName="NDG", CFBundleDisplayName="NDG", CFBundlePackageType="APPL",
        CFBundleShortVersionString=version, CFBundleVersion=build, CFBundleDevelopmentRegion="zh_CN",
        LSApplicationCategoryType="public.app-category.utilities", LSMinimumSystemVersion="13.0",
        NSHighResolutionCapable=True,
    ), out)
PY
icon=cmd/ndg-desktop/build/appicon.png
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$icon" --out "$output/NDG.iconset/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" "$icon" --out "$output/NDG.iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$output/NDG.iconset" -o "$contents/Resources/NDG.icns"
cp LICENSE NOTICE THIRD_PARTY_NOTICES.md "$contents/Resources/"
(
  cd cmd/ndg-desktop/frontend
  npm ci --ignore-scripts
  VITE_NDG_CHANNEL=mas npm run build
)
ldflags="-X github.com/FNB2026/nas-data-governance/internal/version.Version=$version -X github.com/FNB2026/nas-data-governance/internal/version.Channel=mas -X github.com/FNB2026/nas-data-governance/internal/version.Commit=$revision -X github.com/FNB2026/nas-data-governance/internal/version.BuildTime=$stamp"
if [[ "$mode" == distribution || "${NDG_MAS_QA_UNIVERSAL:-0}" == 1 ]]; then
  arches=(arm64 amd64)
else
  arches=("$(go env GOHOSTARCH)")
fi
for arch in "${arches[@]}"; do
  CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" MACOSX_DEPLOYMENT_TARGET=13.0 \
    go build -trimpath -tags 'desktop,production,mas' -ldflags "$ldflags" -o "$output/ndg-$arch" ./cmd/ndg-desktop
done
if [[ "${#arches[@]}" == 2 ]]; then
  lipo -create "$output/ndg-arm64" "$output/ndg-amd64" -output "$contents/MacOS/NDG"
else
  cp "$output/ndg-${arches[0]}" "$contents/MacOS/NDG"
fi
entitlements="$output/entitlements.plist"
cp cmd/ndg-desktop/build/mas/entitlements.plist "$entitlements"
identity=-
if [[ "$mode" == distribution ]]; then
  security cms -D -i "$NDG_MAS_PROFILE" > "$output/profile.plist"
  python3 - "$output/profile.plist" "$entitlements" "$bundle_id" "$NDG_MAS_PRIVACY_MANIFEST" <<'PY'
import datetime, plistlib, sys
profile_path, entitlements_path, bundle, manifest_path = sys.argv[1:]
def read(path):
    with open(path, "rb") as src:
        return plistlib.load(src)
profile = read(profile_path)
granted = profile["Entitlements"]
teams = profile["TeamIdentifier"]
expiry = profile["ExpirationDate"].replace(tzinfo=datetime.timezone.utc)
identifier = granted.get("com.apple.application-identifier")
team = granted.get("com.apple.developer.team-identifier")
if (expiry <= datetime.datetime.now(datetime.timezone.utc) or profile.get("ProvisionedDevices")
        or profile.get("ProvisionsAllDevices") or granted.get("get-task-allow")
        or granted.get("com.apple.security.get-task-allow")
        or team not in teams or identifier != f"{team}.{bundle}"):
    raise SystemExit("Expected a current App Store distribution profile for the exact Bundle ID")
entitlements = read(entitlements_path)
for key in ("com.apple.application-identifier", "com.apple.developer.team-identifier"):
    entitlements[key] = granted[key]
with open(entitlements_path, "wb") as out:
    plistlib.dump(entitlements, out)
manifest = read(manifest_path)
if set(manifest) != {"NSPrivacyTracking", "NSPrivacyTrackingDomains", "NSPrivacyCollectedDataTypes", "NSPrivacyAccessedAPITypes"}:
    raise SystemExit("Privacy manifest is missing expected top-level keys")
PY
  cp "$NDG_MAS_PROFILE" "$contents/embedded.provisionprofile"
  cp "$NDG_MAS_PRIVACY_MANIFEST" "$contents/Resources/PrivacyInfo.xcprivacy"
  identity="$NDG_MAS_APP_IDENTITY"
fi
# umask 077 protects private build inputs, but App Store validation rejects a
# bundle whose files are unreadable to non-root users (error 90255). Restore
# standard bundle permissions before sealing the signature/package.
chmod -R a+rX "$app"
codesign --force --sign "$identity" --entitlements "$entitlements" "$app"
# codesign creates _CodeSignature/CodeResources under the active umask; make
# the sealed bundle readable before App Store validation and packaging.
chmod -R a+rX "$app"
codesign --verify --strict --verbose=2 "$app"
plutil -lint "$contents/Info.plist" "$entitlements"
lipo -archs "$contents/MacOS/NDG"
if [[ "$mode" == distribution ]]; then
  productbuild --component "$app" /Applications --sign "$NDG_MAS_INSTALLER_IDENTITY" "$output/NDG.pkg"
  pkgutil --check-signature "$output/NDG.pkg"
  shasum -a 256 "$output/NDG.pkg"
fi
