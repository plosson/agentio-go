MODULE := github.com/plosson/agentio-go
BIN := bin/agentio

# The version is the nearest git tag (or the commit when there is none).
# Outside a git checkout it is empty, and the binary keeps the default
# version set in internal/cli.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)
LDFLAGS := $(if $(VERSION),-X $(MODULE)/internal/cli.Version=$(VERSION))

# Keep in step with .github/workflows/ci.yml. v0.6.x is the last release
# that supports the Go version in go.mod (1.24).
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1

.PHONY: build test lint fmt golden

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/agentio

test:
	go test -race -count=1 ./...

lint:
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	go run $(STATICCHECK) ./...

fmt:
	gofmt -w .

# Rewrite the Bun goldens (testdata/bun, read through internal/golden) from a
# checkout of the Bun CLI with its dependencies installed (bun install).
# Tests never run Bun otherwise.
golden:
	@if [ -z "$(BUN_REPO)" ]; then echo "usage: make golden BUN_REPO=/path/to/agentio"; exit 2; fi
	AGENTIO_BUN_REPO=$(abspath $(BUN_REPO)) go test -count=1 -timeout 30m ./...
