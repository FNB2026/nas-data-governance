.PHONY: build test fmt vet

build:
	mkdir -p bin
	go build -o bin/nas-governance ./cmd/nas-governance

test:
	go test ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go')

vet:
	go vet ./...
