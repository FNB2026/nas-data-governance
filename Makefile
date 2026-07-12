.PHONY: build test fmt vet release clean-dist

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

# Release: cross-compile darwin/linux arm64/amd64, package tar.gz, SHA256SUMS
release: clean-dist
	@mkdir -p $(DIST_DIR)
	@for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		bin=nas-governance-$${os}-$${arch}; \
		echo "==> Building $${os}/$${arch}"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -trimpath -ldflags "-s -w" -o $(DIST_DIR)/$$bin ./cmd/nas-governance; \
		tar -czf $(DIST_DIR)/$$bin.tar.gz -C $(DIST_DIR) $$bin; \
		rm $(DIST_DIR)/$$bin; \
	done
	@cd $(DIST_DIR) && sha256sum *.tar.gz > SHA256SUMS
	@echo "==> Release artifacts in $(DIST_DIR)/:"
	@ls -lh $(DIST_DIR)/

clean-dist:
	rm -rf $(DIST_DIR)
