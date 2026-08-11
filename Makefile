.PHONY: build test test-policy lint cover cover-check snapshot clean hooks

BINARY := ds
MODULE := github.com/devspecs-com/devspecs-cli
VERSION_PKG := $(MODULE)/internal/version
COVERAGE_FLOOR ?= 80.0

LDFLAGS := -s -w \
	-X $(VERSION_PKG).Version=dev \
	-X $(VERSION_PKG).Commit=$$(git rev-parse --short HEAD 2>/dev/null || echo none) \
	-X $(VERSION_PKG).Date=$$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/ds

ifeq ($(shell go env CGO_ENABLED),1)
RACE := -race
else
RACE :=
endif

test:
	go test $(RACE) -count=1 ./...

test-policy:
	go run ./scripts/ci/check-test-assertions

lint:
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi
	@UNFORMATTED=$$(gofmt -l .); if [ -n "$$UNFORMATTED" ]; then echo "unformatted files:"; echo "$$UNFORMATTED"; exit 1; fi

# One-time per clone: run Git hooks from .githooks/ (pre-commit mirrors CI lint + optional tests).
hooks:
	git config core.hooksPath .githooks
	@echo "Configured core.hooksPath=.githooks for this repository clone."

cover:
	go test $(RACE) -coverprofile coverage.out ./...
	go tool cover -func coverage.out

# Aggregate statement coverage across ./... using exact profile counts. Per-package floors vary.
cover-check:
	go test $(RACE) -coverprofile coverage.out -covermode atomic ./...
	go run ./scripts/ci/check-coverage --profile coverage.out --floor "$(COVERAGE_FLOOR)"

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -rf dist/
