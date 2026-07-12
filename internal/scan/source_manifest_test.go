package scan

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestScan_SourceManifestHiddenByDefault(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "plugins/auth.lua", "local function login() return true end\n")

	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})
	result, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceManifest != nil {
		t.Fatalf("default scan should not emit source manifest diagnostics: %#v", result.SourceManifest)
	}
	repo := db.GetRepoByRoot(repoRoot)
	if repo == nil {
		t.Fatal("repo not recorded")
	}
	counts, err := db.CountSourceManifest(repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Files != 0 {
		t.Fatalf("default scan should not populate source manifest, got %#v", counts)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceManifest == nil {
		t.Fatal("expected source manifest diagnostics")
	}
	if result.SourceManifest.IndexedFiles != 2 || result.SourceManifest.IndexedTests != 1 {
		t.Fatalf("unexpected source manifest diagnostics: %#v", result.SourceManifest)
	}
	if result.Found["source_context"] != 0 {
		t.Fatalf("manifest-only Lua files should not become source_context artifacts, got found=%#v", result.Found)
	}
	repo := db.GetRepoByRoot(repoRoot)
	if repo == nil {
		t.Fatal("repo not recorded")
	}
	counts, err := db.CountSourceManifest(repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Files != 2 || counts.FTSRows != 2 {
		t.Fatalf("unexpected manifest counts: %#v", counts)
	}
	if counts.Symbols == 0 || counts.Tests == 0 || counts.Imports == 0 {
		t.Fatalf("expected compact symbols/tests/imports, got %#v", counts)
	}
	var ftsSymbols string
	if err := db.QueryRow("SELECT symbols FROM source_manifest_fts WHERE path = ?", "plugins/auth.lua").Scan(&ftsSymbols); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ftsSymbols, "logout") {
		t.Fatalf("expected FTS symbols to be capped while structured symbols remain, got %q", ftsSymbols)
	}
	var artifactCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM artifacts WHERE repo_id = ?", repo.ID).Scan(&artifactCount); err != nil {
		t.Fatal(err)
	}
	if artifactCount != 0 {
		t.Fatalf("source manifest should not inflate artifacts, got %d", artifactCount)
	}
}

func TestScan_SourceManifestParallelExtractionIsDeterministic(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "plugins/auth.lua", "local function login() return true end\nlocal function logout() return true end\nrequire('kong.plugins.base')\n")
	writeScanTestFile(t, repoRoot, "plugins/session.lua", "local function session() return true end\nrequire('kong.plugins.auth')\n")
	writeScanTestFile(t, repoRoot, "tests/auth.lua", "describe('auth plugin', function() it('logs in', function() end) end)\n")
	writeScanTestFile(t, repoRoot, "tests/session.lua", "describe('session plugin', function() it('refreshes', function() end) end)\n")

	oneWorker := runSourceManifestSnapshot(t, repoRoot, 1)
	manyWorkers := runSourceManifestSnapshot(t, repoRoot, 4)
	if !reflect.DeepEqual(manyWorkers, oneWorker) {
		t.Fatalf("parallel source manifest extraction changed snapshot:\ngot:  %#v\nwant: %#v", manyWorkers, oneWorker)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceManifest == nil {
		t.Fatal("expected source manifest diagnostics")
	}
	if result.SourceManifest.IndexedFiles != 2 || result.SourceManifest.IndexedTests != 1 {
		t.Fatalf("expected role-balanced module-root cap, got %#v", result.SourceManifest)
	}
	if got := result.SourceManifest.IgnoredByReason["module_root_cap"]; got != 2 {
		t.Fatalf("expected 2 module_root_cap skips, got %d in %#v", got, result.SourceManifest.IgnoredByReason)
	}
	repo := db.GetRepoByRoot(repoRoot)
	counts, err := db.CountSourceManifest(repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Files != 2 {
		t.Fatalf("expected capped manifest rows, got %#v", counts)
	}
}

func runSourceManifestSnapshot(t *testing.T, repoRoot string, workers int) []string {
	t.Helper()
	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})
	if _, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:  true,
		SourceManifest:  true,
		FileWorkerCount: workers,
	}); err != nil {
		t.Fatal(err)
	}
	return sourceManifestSnapshot(t, db)
}

func sourceManifestSnapshot(t *testing.T, db *store.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT sm.path, sm.content_hash, sm.language, sm.source_root, sm.source_role,
		COALESCE(fts.symbols,''), COALESCE(fts.test_names,''), COALESCE(fts.imports,'')
		FROM source_manifest sm
		LEFT JOIN source_manifest_fts fts ON fts.file_id = sm.file_id
		ORDER BY sm.path`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var path, hash, language, root, role, symbols, tests, imports string
		if err := rows.Scan(&path, &hash, &language, &root, &role, &symbols, &tests, &imports); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.Join([]string{path, hash, language, root, role, symbols, tests, imports}, "\x00"))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSourceManifestModuleRootLimitIndexesSmallRootsFully(t *testing.T) {
	oldSoftFull := sourceManifestModuleRootSoftFullFiles
	sourceManifestModuleRootSoftFullFiles = 5
	t.Cleanup(func() { sourceManifestModuleRootSoftFullFiles = oldSoftFull })

	if got := sourceManifestModuleRootCandidateLimit(5); got != 5 {
		t.Fatalf("small module roots should be fully indexed, got limit %d", got)
	}
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
	if skipped != 2 || len(selected) != 4 {
		t.Fatalf("unexpected selected/skipped: selected=%#v skipped=%d", selected, skipped)
	}
	roots := map[string]bool{}
	for _, candidate := range selected {
		roots[candidate.root.path] = true
	}
	for _, want := range []string{"sdk/a", "sdk/b", "sdk/c"} {
		if !roots[want] {
			t.Fatalf("expected capped selection to seed root %s, got %#v", want, selected)
		}
	}
}

func TestScan_SourceManifestRescanReplacesRows(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "plugins/auth.lua", "local function login() return true end\n")

	db := openScanManifestTestDB(t)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}})
	if _, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true}); err != nil {
		t.Fatal(err)
	}
	writeScanTestFile(t, repoRoot, "plugins/session.lua", "local function session() return true end\n")
	if _, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true, SourceManifest: true}); err != nil {
		t.Fatal(err)
	}
	repo := db.GetRepoByRoot(repoRoot)
	counts, err := db.CountSourceManifest(repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Files != 2 || counts.FTSRows != 2 {
		t.Fatalf("rescan should replace, not duplicate, got %#v", counts)
	}
}

func TestSourceManifestImportExtraction(t *testing.T) {
	got := extractSourceManifestImports(`
import React from "react"
const fs = require("fs")
from pathlib import Path
use crate::session::Token
#include <stdio.h>
`)
	want := map[string]bool{"React": true, "fs": true, "pathlib": true, "crate::session::Token": true, "stdio.h": true}
	for _, value := range got {
		delete(want, value)
	}
	if len(want) != 0 {
		t.Fatalf("missing imports: %#v; got %#v", want, got)
	}
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
	if len(got) != sourceManifestMaxImportsPerFile {
		t.Fatalf("expected cap %d, got %d: %#v", sourceManifestMaxImportsPerFile, len(got), got)
	}
	for _, want := range []string{"./local", "internal/auth", "pkg/config"} {
		found := false
		for _, value := range got {
			if value == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing preferred local import %q in %#v", want, got)
		}
	}
	for _, unexpected := range []string{"fmt", "net/http", "react"} {
		for _, value := range got {
			if value == unexpected {
				t.Fatalf("low-signal import %q should be displaced by local imports: %#v", unexpected, got)
			}
		}
	}
}

func writeScanTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openScanManifestTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
