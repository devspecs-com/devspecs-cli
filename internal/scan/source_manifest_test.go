package scan

import (
	"context"
	"os"
	"path/filepath"
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
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "plugins/auth.lua", "local function login() return true end\nrequire('kong.plugins.base')\n")
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
	var artifactCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM artifacts WHERE repo_id = ?", repo.ID).Scan(&artifactCount); err != nil {
		t.Fatal(err)
	}
	if artifactCount != 0 {
		t.Fatalf("source manifest should not inflate artifacts, got %d", artifactCount)
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
