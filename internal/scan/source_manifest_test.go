package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScan_SourceManifestHiddenByDefault(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "plugins/auth.lua", "local function login() return true end\n")

	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})
	result, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true})
	require.NoError(t, err)
	require.Nil(t, result.SourceManifest,
		"default scan should not emit source manifest diagnostics: %#v", result.SourceManifest)

	repo := db.GetRepoByRoot(repoRoot)
	require.NotNil(t, repo,
		"repo not recorded")

	counts, err := db.CountSourceManifest(repo.ID)
	require.NoError(t, err)
	require.Equal(t, 0, counts.Files,
		"default scan should not populate source manifest, got %#v", counts)

}

func TestScan_SourceManifestPopulatesCompactRowsWithoutArtifacts(t *testing.T) {
	oldFTSSymbolCap := sourceManifestMaxFTSSymbolsPerFile
	sourceManifestMaxFTSSymbolsPerFile = 1
	t.Cleanup(func() { sourceManifestMaxFTSSymbolsPerFile = oldFTSSymbolCap })

	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "plugins/auth.lua", "local function login() return true end\nlocal function logout() return true end\nrequire('kong.plugins.base')\n")
	writeScanTestFile(t, repoRoot, "tests/auth.lua", "describe('auth plugin', function() it('logs in', function() end) end)\n")

	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})
	result, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction: true,
		SourceManifest: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result.SourceManifest,
		"expected source manifest diagnostics")
	assert.Equal(t, 2, result.SourceManifest.IndexedFiles)
	assert.Equal(t, 1, result.SourceManifest.IndexedTests)
	assert.Zero(t, result.Found["source_context"])

	repo := db.GetRepoByRoot(repoRoot)
	require.NotNil(t, repo,
		"repo not recorded")

	counts, err := db.CountSourceManifest(repo.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Files)
	assert.Equal(t, 2, counts.FTSRows)
	assert.Positive(t, counts.Symbols)
	assert.Positive(t, counts.Tests)
	assert.Positive(t, counts.Imports)

	var ftsSymbols string
	require.NoError(t, db.QueryRow("SELECT symbols FROM source_manifest_fts WHERE path = ?", "plugins/auth.lua").Scan(&ftsSymbols))
	assert.NotContains(t, ftsSymbols, "logout")

	var artifactCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifacts WHERE repo_id = ?", repo.ID).Scan(&artifactCount))
	assert.Zero(t, artifactCount)
}

func TestScan_SourceManifestAdmitsShellExtensionsFromFirstPartyRoots(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "lib/providers/adapter.sh", "#!/bin/sh\necho adapter\n")
	writeScanTestFile(t, repoRoot, "scripts/verify.bash", "#!/usr/bin/env bash\necho verify\n")
	writeScanTestFile(t, repoRoot, "tests/smoke.zsh", "#!/usr/bin/env zsh\necho smoke\n")
	writeScanTestFile(t, repoRoot, "lib/generated/ignored.sh", "#!/bin/sh\necho generated\n")
	writeScanTestFile(t, repoRoot, "vendor/ignored.sh", "#!/bin/sh\necho vendor\n")
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	require.NoError(t, err)
	snapshot := sourceManifestSnapshot(t, db)

	require.Len(t, snapshot, 3)
	assert.Contains(t, snapshot[0], "lib/providers/adapter.sh\x00")
	assert.Contains(t, snapshot[0], "\x00shell\x00lib\x00implementation\x00")
	assert.Contains(t, snapshot[1], "scripts/verify.bash\x00")
	assert.Contains(t, snapshot[1], "\x00shell\x00scripts\x00test\x00")
	assert.Contains(t, snapshot[2], "tests/smoke.zsh\x00")
	assert.Contains(t, snapshot[2], "\x00shell\x00tests\x00test\x00")
}

func TestScan_SourceManifestAdmitsBatsAsShellBehaviorTest(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "test/formatter.bats", "@test \"formats TAP output\" {\n  true\n}\n")
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	require.NoError(t, err)
	snapshot := sourceManifestSnapshot(t, db)

	require.Len(t, snapshot, 1)
	assert.Contains(t, snapshot[0], "test/formatter.bats\x00")
	assert.Contains(t, snapshot[0], "\x00shell\x00test\x00test\x00")
	assert.Contains(t, snapshot[0], "formats TAP output")
}

func TestScan_SourceManifestPersistsShellSemanticRelationshipsAndRanges(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "bin/tool.sh", "source \"$ROOT/lib/core.sh\"\ncmd_spawn() { true; }\ncase \"$1\" in\n  spawn) cmd_spawn \"$@\" ;;\nesac\n")
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	require.NoError(t, err)
	var symbol, kind, parent, ftsSymbols string
	var line, endLine int
	require.NoError(t, db.QueryRow(`SELECT s.symbol, s.kind, s.parent, s.line, s.end_line, f.symbols
		FROM source_manifest_symbols s
		JOIN source_manifest_fts f ON f.file_id = s.file_id
		WHERE s.kind = 'dispatch'`).Scan(&symbol, &kind, &parent, &line, &endLine, &ftsSymbols))
	assert.Equal(t, "spawn", symbol)
	assert.Equal(t, "dispatch", kind)
	assert.Equal(t, "cmd_spawn", parent)
	assert.Equal(t, 4, line)
	assert.Equal(t, 4, endLine)
	assert.Contains(t, ftsSymbols, "spawn -> cmd_spawn (line 4)")
}

func TestScan_SourceManifestPersistsShellImportRanges(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "bin/tool.sh", "source \"$ROOT/lib/core.sh\"\n")
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	require.NoError(t, err)
	var importRef, ftsImports string
	var line, endLine int
	require.NoError(t, db.QueryRow(`SELECT i.import_ref, i.line, i.end_line, f.imports
		FROM source_manifest_imports i
		JOIN source_manifest_fts f ON f.file_id = i.file_id`).Scan(&importRef, &line, &endLine, &ftsImports))
	assert.Equal(t, "$ROOT/lib/core.sh", importRef)
	assert.Equal(t, 1, line)
	assert.Equal(t, 1, endLine)
	assert.Contains(t, ftsImports, "$ROOT/lib/core.sh (line 1)")
}

func TestScan_SourceManifestWithInvalidShell_DoesNotPersistFallbackImports(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "bin/tool.sh", "source ./lib/core.sh\nif then\n")
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, scanErr := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	var importCount int
	queryErr := db.QueryRow(`SELECT COUNT(*) FROM source_manifest_imports`).Scan(&importCount)

	require.NoError(t, scanErr)
	require.NoError(t, queryErr)
	assert.Zero(t, importCount)
}

func TestScan_SourceManifestAdmitsTrackedExecutableShellEntrypoint(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "bin/tool", "#!/usr/bin/env bash\necho tool\n")
	runGitCommand(t, repoRoot, "init")
	runGitCommand(t, repoRoot, "add", "bin/tool")
	runGitCommand(t, repoRoot, "update-index", "--chmod=+x", "bin/tool")
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	require.NoError(t, err)
	snapshot := sourceManifestSnapshot(t, db)

	require.Len(t, snapshot, 1)
	assert.Contains(t, snapshot[0], "bin/tool\x00")
	assert.Contains(t, snapshot[0], "\x00shell\x00bin\x00implementation\x00")
}

func TestScan_SourceManifestRejectsNonExecutableExtensionlessShellFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "scripts/helper", "#!/bin/sh\necho helper\n")
	runGitCommand(t, repoRoot, "init")
	runGitCommand(t, repoRoot, "add", "scripts/helper")
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	require.NoError(t, err)
	snapshot := sourceManifestSnapshot(t, db)

	assert.Empty(t, snapshot)
}

func TestScan_SourceManifestRejectsExecutableExtensionlessNonShellFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "bin/tool", "#!/usr/bin/env python3\nprint('tool')\n")
	runGitCommand(t, repoRoot, "init")
	runGitCommand(t, repoRoot, "add", "bin/tool")
	runGitCommand(t, repoRoot, "update-index", "--chmod=+x", "bin/tool")
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	require.NoError(t, err)
	snapshot := sourceManifestSnapshot(t, db)

	assert.Empty(t, snapshot)
}

func TestScan_SourceManifestExtractionWithOneWorkerProducesCanonicalSnapshot(t *testing.T) {
	repoRoot := t.TempDir()
	seedSourceManifestParallelFixture(t, repoRoot)
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:  true,
		SourceManifest:  true,
		FileWorkerCount: 1,
	})
	require.NoError(t, err)
	snapshot := sourceManifestSnapshot(t, db)

	assertCanonicalSourceManifestSnapshot(t, snapshot)
}

func TestScan_SourceManifestExtractionWithFourWorkersProducesCanonicalSnapshot(t *testing.T) {
	repoRoot := t.TempDir()
	seedSourceManifestParallelFixture(t, repoRoot)
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:  true,
		SourceManifest:  true,
		FileWorkerCount: 4,
	})
	require.NoError(t, err)
	snapshot := sourceManifestSnapshot(t, db)

	assertCanonicalSourceManifestSnapshot(t, snapshot)
}

func TestScan_SourceManifestCapsNestedModuleRootRows(t *testing.T) {
	oldSoftFull := sourceManifestModuleRootSoftFullFiles
	oldMin := sourceManifestMinModuleRootFilesPerRepo
	oldMax := sourceManifestMaxModuleRootFilesPerRepo
	oldPercent := sourceManifestModuleRootBudgetPercent
	oldSeed := sourceManifestModuleRootSeedFilesPerRoot
	sourceManifestModuleRootSoftFullFiles = 0
	sourceManifestMinModuleRootFilesPerRepo = 2
	sourceManifestMaxModuleRootFilesPerRepo = 2
	sourceManifestModuleRootBudgetPercent = 100
	sourceManifestModuleRootSeedFilesPerRoot = 2
	t.Cleanup(func() {
		sourceManifestModuleRootSoftFullFiles = oldSoftFull
		sourceManifestMinModuleRootFilesPerRepo = oldMin
		sourceManifestMaxModuleRootFilesPerRepo = oldMax
		sourceManifestModuleRootBudgetPercent = oldPercent
		sourceManifestModuleRootSeedFilesPerRoot = oldSeed
	})

	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "sdk/storage/blob/go.mod", "module example.com/sdk/storage/blob\n")
	writeScanTestFile(t, repoRoot, "sdk/storage/blob/client.go", "package blob\nfunc Client() {}\n")
	writeScanTestFile(t, repoRoot, "sdk/storage/blob/server.go", "package blob\nfunc Server() {}\n")
	writeScanTestFile(t, repoRoot, "sdk/storage/blob/client_test.go", "package blob\nfunc TestClient(t *testing.T) {}\n")
	writeScanTestFile(t, repoRoot, "sdk/storage/blob/server_test.go", "package blob\nfunc TestServer(t *testing.T) {}\n")

	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})
	result, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction: true,
		SourceManifest: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result.SourceManifest,
		"expected source manifest diagnostics")
	assert.Equal(t, 2, result.SourceManifest.IndexedFiles)
	assert.Equal(t, 1, result.SourceManifest.IndexedTests)
	assert.Equal(t, 2, result.SourceManifest.IgnoredByReason["module_root_cap"])

	repo := db.GetRepoByRoot(repoRoot)
	require.NotNil(t, repo)
	counts, err := db.CountSourceManifest(repo.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Files)
}

func sourceManifestSnapshot(t *testing.T, db *store.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT sm.path, sm.content_hash, sm.language, sm.source_root, sm.source_role,
		COALESCE(fts.symbols,''), COALESCE(fts.test_names,''), COALESCE(fts.imports,'')
		FROM source_manifest sm
		LEFT JOIN source_manifest_fts fts ON fts.file_id = sm.file_id
		ORDER BY sm.path`)
	require.NoError(t, err)

	defer rows.Close()
	var out []string
	for rows.Next() {
		var path, hash, language, root, role, symbols, tests, imports string
		require.NoError(t, rows.Scan(&path, &hash, &language, &root, &role, &symbols, &tests, &imports))

		out = append(out, strings.Join([]string{path, hash, language, root, role, symbols, tests, imports}, "\x00"))
	}
	require.NoError(t, rows.Err())

	return out
}

func TestSourceManifestModuleRootLimitIndexesSmallRootsFully(t *testing.T) {
	oldSoftFull := sourceManifestModuleRootSoftFullFiles
	sourceManifestModuleRootSoftFullFiles = 5
	t.Cleanup(func() { sourceManifestModuleRootSoftFullFiles = oldSoftFull })
	got := sourceManifestModuleRootCandidateLimit(5)

	assert.Equal(t, 5, got)

}

func TestSourceManifestModuleRootCapSeedsMultipleRoots(t *testing.T) {
	oldSoftFull := sourceManifestModuleRootSoftFullFiles
	oldMin := sourceManifestMinModuleRootFilesPerRepo
	oldMax := sourceManifestMaxModuleRootFilesPerRepo
	oldPercent := sourceManifestModuleRootBudgetPercent
	oldSeed := sourceManifestModuleRootSeedFilesPerRoot
	sourceManifestModuleRootSoftFullFiles = 0
	sourceManifestMinModuleRootFilesPerRepo = 4
	sourceManifestMaxModuleRootFilesPerRepo = 4
	sourceManifestModuleRootBudgetPercent = 100
	sourceManifestModuleRootSeedFilesPerRoot = 1
	t.Cleanup(func() {
		sourceManifestModuleRootSoftFullFiles = oldSoftFull
		sourceManifestMinModuleRootFilesPerRepo = oldMin
		sourceManifestMaxModuleRootFilesPerRepo = oldMax
		sourceManifestModuleRootBudgetPercent = oldPercent
		sourceManifestModuleRootSeedFilesPerRoot = oldSeed
	})

	candidates := []sourceManifestCandidate{
		{rel: "sdk/a/client.go", root: firstPartySourceRoot{path: "sdk/a", kind: "module_root"}, role: "implementation"},
		{rel: "sdk/a/server.go", root: firstPartySourceRoot{path: "sdk/a", kind: "module_root"}, role: "implementation"},
		{rel: "sdk/b/client.go", root: firstPartySourceRoot{path: "sdk/b", kind: "module_root"}, role: "implementation"},
		{rel: "sdk/b/server.go", root: firstPartySourceRoot{path: "sdk/b", kind: "module_root"}, role: "implementation"},
		{rel: "sdk/c/client.go", root: firstPartySourceRoot{path: "sdk/c", kind: "module_root"}, role: "implementation"},
		{rel: "sdk/c/server.go", root: firstPartySourceRoot{path: "sdk/c", kind: "module_root"}, role: "implementation"},
	}
	selected, skipped := capSourceManifestModuleRootCandidates(candidates, sourceManifestModuleRootCandidateLimit(len(candidates)))
	assert.Equal(t, 2, skipped)
	require.Len(t, selected, 4)

	roots := map[string]bool{}
	for _, candidate := range selected {
		roots[candidate.root.path] = true
	}
	require.Len(t, roots, 3)
	assert.True(t, roots["sdk/a"])
	assert.True(t, roots["sdk/b"])
	assert.True(t, roots["sdk/c"])
}

func TestScan_SourceManifestRescanReplacesRows(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "plugins/auth.lua", "local function login() return true end\n")

	writeScanTestFile(t, repoRoot, "plugins/session.lua", "local function session() return true end\n")
	db := openScanManifestTestDB(t)
	now := "2026-08-11T00:00:00Z"
	mustExecScanManifestSQL(t, db, `INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)`, "repo_existing", repoRoot, now, now)
	mustExecScanManifestSQL(t, db, `INSERT INTO source_manifest (
		file_id, repo_id, path, content_hash, size_bytes, language, source_root,
		source_root_kind, source_role, first_party_score, ignored_reason, indexed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"stale_file", "repo_existing", "plugins/stale.lua", "old", 1, "lua", "plugins", "common_root", "implementation", 1, "", now)
	mustExecScanManifestSQL(t, db, `INSERT INTO source_manifest_fts (
		file_id, path, path_terms, source_root, language, source_role, symbols, test_names, imports
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"stale_file", "plugins/stale.lua", "plugins stale lua", "plugins", "lua", "implementation", "stale", "", "")
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true})
	require.NoError(t, err)

	repo := db.GetRepoByRoot(repoRoot)
	require.NotNil(t, repo)
	counts, err := db.CountSourceManifest(repo.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Files)
	assert.Equal(t, 2, counts.FTSRows)
}

func TestSourceManifestImportExtraction(t *testing.T) {
	got := extractSourceManifestImports(`
import React from "react"
const fs = require("fs")
from pathlib import Path
use crate::session::Token
#include <stdio.h>
`)
	require.Len(t, got, 6)
	assert.Equal(t, "React", got[0])
	assert.Equal(t, "pathlib", got[1])
	assert.Equal(t, "fs", got[2])
	assert.Equal(t, "react", got[3])
	assert.Equal(t, "crate::session::Token", got[4])
	assert.Equal(t, "stdio.h", got[5])

}

func TestSourceManifestImportCompactionPrefersLocalAndCaps(t *testing.T) {
	got := compactSourceManifestImports([]string{
		"fmt",
		"net/http",
		"internal/auth",
		"pkg/config",
		"sdk/storage/blob",
		"crate::session::token",
		"./local",
		"react",
		"services/billing",
		"org.example.External",
		"apps/admin",
		"components/button",
	})
	require.Len(t, got, sourceManifestMaxImportsPerFile,
		"expected cap %d, got %d: %#v", sourceManifestMaxImportsPerFile, len(got), got)

	assert.Equal(t, "./local", got[0])
	assert.Equal(t, "apps/admin", got[1])
	assert.Equal(t, "components/button", got[2])
	assert.Equal(t, "crate::session::token", got[3])
	assert.Equal(t, "internal/auth", got[4])
	assert.Equal(t, "pkg/config", got[5])
}

func writeScanTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func openScanManifestTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func seedSourceManifestParallelFixture(t *testing.T, repoRoot string) {
	t.Helper()

	writeScanTestFile(t, repoRoot, "plugins/auth.lua", "local function login() return true end\nlocal function logout() return true end\nrequire('kong.plugins.base')\n")
	writeScanTestFile(t, repoRoot, "plugins/session.lua", "local function session() return true end\nrequire('kong.plugins.auth')\n")
	writeScanTestFile(t, repoRoot, "tests/auth.lua", "describe('auth plugin', function() it('logs in', function() end) end)\n")
	writeScanTestFile(t, repoRoot, "tests/session.lua", "describe('session plugin', function() it('refreshes', function() end) end)\n")
}

func assertCanonicalSourceManifestSnapshot(t *testing.T, snapshot []string) {
	t.Helper()

	require.Len(t, snapshot, 4)
	assert.Contains(t, snapshot[0], "plugins/auth.lua\x00")
	assert.Contains(t, snapshot[1], "plugins/session.lua\x00")
	assert.Contains(t, snapshot[2], "tests/auth.lua\x00")
	assert.Contains(t, snapshot[3], "tests/session.lua\x00")
}

func mustExecScanManifestSQL(t *testing.T, db *store.DB, query string, args ...any) {
	t.Helper()

	_, err := db.Exec(query, args...)
	require.NoError(t, err)
}
