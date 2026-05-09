.PHONY: build test lint cover snapshot clean

BINARY := ds
MODULE := github.com/devspecs-com/devspecs-cli
VERSION_PKG := $(MODULE)/internal/version

LDFLAGS := -s -w \
	-X $(VERSION_PKG).Version=dev \
	-X $(VERSION_PKG).Commit=$$(git rev-parse --short HEAD 2>/dev/null || echo none) \
	-X $(VERSION_PKG).Date=$$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/ds

RACE := $(shell go env CGO_ENABLED 2>/dev/null | grep -q 1 && echo "-race")

test:
	go test $(RACE) -count=1 ./...

lint:
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi
	@UNFORMATTED=$$(gofmt -l .); if [ -n "$$UNFORMATTED" ]; then echo "unformatted files:"; echo "$$UNFORMATTED"; exit 1; fi

cover:
	go test $(RACE) -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -rf dist/
