#!/bin/sh
# CI-style checks for local commits. Mirrors .github/workflows/go.yml lint steps.
# Tests are optional (CI always runs full suite with -race).
#
# DEVSPECS_PRECOMMIT_TESTS:
#   quick (default) — go test -count=1 ./...  (no -race, faster)
#   ci | full | race — same as CI unit test step (requires CGO for -race on some platforms)
#   0 | false | skip | none — skip tests

set -e
cd "$(git rev-parse --show-toplevel)"

echo "pre-commit: go vet ./..."
go vet ./...

if command -v staticcheck >/dev/null 2>&1; then
	echo "pre-commit: staticcheck ./..."
	staticcheck ./...
else
	echo "pre-commit: warning: staticcheck not in PATH; install: go install honnef.co/go/tools/cmd/staticcheck@latest"
	echo "pre-commit: (skipping staticcheck — CI will still run it)"
fi

echo "pre-commit: gofmt -l ."
UNFORMATTED=$(gofmt -l .)
if [ -n "$UNFORMATTED" ]; then
	echo "unformatted files (fix with: gofmt -w .):"
	echo "$UNFORMATTED"
	exit 1
fi

case "${DEVSPECS_PRECOMMIT_TESTS:-quick}" in
0 | false | skip | none)
	echo "pre-commit: skipping go test (DEVSPECS_PRECOMMIT_TESTS=$DEVSPECS_PRECOMMIT_TESTS)"
	;;
ci | full | race)
	echo "pre-commit: go test -race -count=1 ./... (CI-like)"
	go test -race -count=1 ./...
	;;
*)
	echo "pre-commit: go test -count=1 ./... (set DEVSPECS_PRECOMMIT_TESTS=ci for -race)"
	go test -count=1 ./...
	;;
esac

echo "pre-commit: OK"
