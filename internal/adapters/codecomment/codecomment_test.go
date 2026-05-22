package codecomment

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

func TestExtractComments_IntentOnly(t *testing.T) {
	body := `package billing

// Copyright 2026 Example Inc.
// Licensed under the Apache License.

// increment the counter
counter++

// Invariant: stripe_event_id must always be checked before applying credits.
func applyCredit() {}

// TODO: remove the legacy retry workaround after the migration finishes.
func retry() {}
`
	units := extractComments("billing/webhook.go", body)
	if len(units) != 2 {
		t.Fatalf("extractComments returned %d units, want 2: %#v", len(units), units)
	}
	if units[0].Role != "invariant" {
		t.Fatalf("first role = %q, want invariant", units[0].Role)
	}
	if units[1].Role != "todo" {
		t.Fatalf("second role = %q, want todo", units[1].Role)
	}
}

func TestDiscoverRequiresConfigAndParsesArtifact(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "billing", "webhook.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package billing\n\n// Invariant: stripe_event_id must always be checked before applying credits.\nfunc applyCredit() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := &Adapter{}
	disabled, err := adapter.Discover(context.Background(), root, config.DefaultRepoConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 0 {
		t.Fatalf("disabled code comments returned %d candidates", len(disabled))
	}

	cfg := config.WithCodeCommentArtifacts(config.DefaultRepoConfig(), true)
	candidates, err := adapter.Discover(context.Background(), root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("Discover returned %d candidates, want 1: %#v", len(candidates), candidates)
	}
	art, sources, _, err := adapter.Parse(context.Background(), candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if art.Kind != config.KindSourceContext || art.Subtype != config.SubtypeCodeComment {
		t.Fatalf("kind/subtype = %q/%q", art.Kind, art.Subtype)
	}
	if art.Extracted["mode"] != "intent" || art.Extracted["comment_role"] != "invariant" {
		t.Fatalf("extracted = %#v", art.Extracted)
	}
	if len(sources) != 1 || sources[0].SourceType != "code_comment" {
		t.Fatalf("sources = %#v", sources)
	}
}
