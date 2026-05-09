// Package version holds build metadata injected by GoReleaser (-ldflags -X).
package version

// Version is the release tag (e.g. v0.1.0) or "dev" when built locally.
var Version = "dev"

// Commit is the git SHA at build time.
var Commit = "none"

// Date is the build timestamp (RFC3339).
var Date = "unknown"
