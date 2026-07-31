.PHONY: build test fmt vet public-check release-check release clean-dist frontend-test frontend-build frontend-check desktop desktop-dev desktop-build wails-check version-check desktop-sign desktop-dmg desktop-notarize desktop-release desktop-verify

# VERSION is the single source of truth, read from the VERSION file.
# CLI builds fall back to git describe for legacy compatibility.
VERSION_FILE := $(shell cat VERSION 2>/dev/null)
VERSION ?= $(if $(VERSION_FILE),$(VERSION_FILE),$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
# CHANNEL is derived from the version pre-release identifier.
# 0.5.0-beta.1 → beta, 0.5.0-alpha.1 → alpha, 0.5.0 → stable, dev → dev
CHANNEL ?= $(shell echo $(VERSION) | sed -n 's/^[0-9][0-9.]*-\([a-z]*\)\.[0-9]*/\1/p; s/^[0-9][0-9.]*$$/stable/p; s/^dev$$/dev/p')
DIST_DIR ?= dist
VERSION_PKG = github.com/FNB2026/nas-data-governance/internal/version
LDFLAGS = -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME) -X $(VERSION_PKG).Channel=$(CHANNEL)

build:
	mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/nas-governance ./cmd/nas-governance

test:
	go test ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go')

vet:
	go vet ./...

public-check:
	./scripts/check-public-boundary.sh

release-check: public-check
	@test -s LICENSE || (echo "release-check: root LICENSE is required" >&2; exit 1)
	@test -s LICENSE-DOCS.md || (echo "release-check: LICENSE-DOCS.md is required" >&2; exit 1)
	@test -s NOTICE || (echo "release-check: NOTICE is required" >&2; exit 1)
	@test -s THIRD_PARTY_NOTICES.md || (echo "release-check: THIRD_PARTY_NOTICES.md is required" >&2; exit 1)

# Release: cross-compile darwin/linux arm64/amd64, package tar.gz, SHA256SUMS
release: release-check clean-dist
	@mkdir -p $(DIST_DIR)
	@cp README.md LICENSE LICENSE-DOCS.md NOTICE THIRD_PARTY_NOTICES.md $(DIST_DIR)/
	@./scripts/collect-third-party-licenses.sh $(DIST_DIR)/third-party-licenses
	@for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		bin=nas-governance-$${os}-$${arch}; \
		echo "==> Building $${os}/$${arch}"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o $(DIST_DIR)/$$bin ./cmd/nas-governance; \
		tar -czf $(DIST_DIR)/$$bin.tar.gz -C $(DIST_DIR) \
			$$bin README.md LICENSE LICENSE-DOCS.md NOTICE THIRD_PARTY_NOTICES.md third-party-licenses; \
		rm $(DIST_DIR)/$$bin; \
	done
	@cd $(DIST_DIR) && if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum *.tar.gz > SHA256SUMS; \
	else \
		shasum -a 256 *.tar.gz > SHA256SUMS; \
	fi
	@echo "==> Release artifacts in $(DIST_DIR)/:"
	@ls -lh $(DIST_DIR)/

clean-dist:
	rm -rf $(DIST_DIR)

# ---- Frontend verification targets ----

FRONTEND_DIR = cmd/ndg-desktop/frontend

frontend-test:
	cd $(FRONTEND_DIR) && npm test

frontend-build:
	cd $(FRONTEND_DIR) && npm run build

frontend-check: frontend-test frontend-build

# ---- Desktop (Wails) targets ----
# Wails CLI v2.13.0 is the fixed toolchain version. Do NOT use @latest.
# Install: go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0

DESKTOP_DIR = cmd/ndg-desktop
WAILS_VERSION := v2.13.0

# wails-check verifies that the Wails CLI in PATH matches the pinned version.
# All desktop targets depend on this to prevent version drift.
wails-check:
	@command -v wails >/dev/null 2>&1 || { \
		echo "wails-check: wails CLI not found in PATH" >&2; \
		echo "  Install: go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)" >&2; \
		exit 1; \
	}
	@actual="$$(wails version 2>/dev/null | head -1 | tr -d '[:space:]')"; \
	if [ "$$actual" != "$(WAILS_VERSION)" ]; then \
		echo "wails-check: installed wails version is '$$actual', expected '$(WAILS_VERSION)'" >&2; \
		echo "  Install: go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)" >&2; \
		exit 1; \
	fi
	@echo "wails-check: OK — $(WAILS_VERSION)"

# version-check runs the release consistency script (allows untagged builds).
version-check:
	./scripts/release/check-version-consistency.sh --allow-untagged

desktop-dev: wails-check
	cd $(DESKTOP_DIR) && wails dev

desktop-build: wails-check
	cd $(DESKTOP_DIR) && wails build \
	  -clean \
	  -trimpath \
	  -platform darwin/arm64 \
	  -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME) -X $(VERSION_PKG).Channel=$(CHANNEL)"

desktop: desktop-build

# ---- macOS distribution targets ----
# These targets handle signing, notarization, and DMG creation.
# They require a macOS host with Developer ID certificates.
# For local testing without Developer ID, use: make desktop-sign AD_HOC=true

# desktop-sign: Sign the .app bundle with Hardened Runtime + entitlements.
# Pass AD_HOC=true for local testing (no Developer ID required).
desktop-sign: desktop-build
	@if [[ "$(AD_HOC)" == "true" ]]; then \
		./scripts/release/sign-macos-app.sh --ad-hoc; \
	else \
		./scripts/release/sign-macos-app.sh; \
	fi

# desktop-dmg: Create a distributable DMG from the signed .app.
# Depends on desktop-sign to ensure the .app is signed first.
desktop-dmg: desktop-sign
	@if [[ "$(AD_HOC)" == "true" ]]; then \
		./scripts/release/create-dmg.sh --ad-hoc; \
	else \
		./scripts/release/create-dmg.sh; \
	fi

# desktop-notarize: Submit the signed DMG to Apple for notarization,
# wait for approval, then staple the ticket.
# Requires notarization credentials (see notarize-macos-app.sh --help).
# NOTARY_PROFILE env var or --keychain-profile arg is the recommended method.
desktop-notarize: desktop-dmg
	@if [[ -n "$(NOTARY_PROFILE)" ]]; then \
		./scripts/release/notarize-macos-app.sh --keychain-profile "$(NOTARY_PROFILE)"; \
	else \
		./scripts/release/notarize-macos-app.sh; \
	fi

# desktop-verify: Verify all macOS release artifacts.
# Checks .app signature, Hardened Runtime, entitlements, DMG signature,
# notarization staple, Gatekeeper assessment, and checksums.
# Use APP_ONLY=true to skip DMG checks (e.g. after ad-hoc sign).
desktop-verify:
	@if [[ "$(APP_ONLY)" == "true" ]]; then \
		./scripts/release/verify-macos-release.sh --app-only; \
	else \
		./scripts/release/verify-macos-release.sh; \
	fi

# desktop-release: Full macOS release pipeline.
# Version-check → Build → Sign → DMG → Notarize → Staple → Verify.
# version-check is a mandatory gate before any production release.
# This is the one-command release path for production builds.
desktop-release: version-check desktop-notarize
	./scripts/release/verify-macos-release.sh
	@echo ""
	@echo "==> macOS release artifacts in $(DIST_DIR)/:"
	@ls -lh $(DIST_DIR)/*.dmg* 2>/dev/null || echo "  (no DMG artifacts found)"
