.PHONY: build test fmt vet public-check release-check release clean-dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DIST_DIR ?= dist

build:
	mkdir -p bin
	go build -o bin/nas-governance ./cmd/nas-governance

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
			go build -trimpath -ldflags "-s -w" -o $(DIST_DIR)/$$bin ./cmd/nas-governance; \
		tar -czf $(DIST_DIR)/$$bin.tar.gz -C $(DIST_DIR) \
			$$bin README.md LICENSE LICENSE-DOCS.md NOTICE THIRD_PARTY_NOTICES.md third-party-licenses; \
		rm $(DIST_DIR)/$$bin; \
	done
	@cd $(DIST_DIR) && sha256sum *.tar.gz > SHA256SUMS
	@echo "==> Release artifacts in $(DIST_DIR)/:"
	@ls -lh $(DIST_DIR)/

clean-dist:
	rm -rf $(DIST_DIR)
