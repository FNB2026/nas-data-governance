.PHONY: build test fmt vet public-check release-check release clean-dist frontend-test frontend-build frontend-check desktop desktop-dev desktop-build

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DIST_DIR ?= dist
VERSION_PKG = github.com/FNB2026/nas-data-governance/internal/version
LDFLAGS = -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)

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
# Requires wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@latest

DESKTOP_DIR = cmd/ndg-desktop

desktop-dev:
	cd $(DESKTOP_DIR) && wails dev

desktop-build:
	cd $(DESKTOP_DIR) && wails build

desktop: desktop-build
