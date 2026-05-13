package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalCommand_TextOutputLabelsRetrieverAndTokenCounter(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented")})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Fixture version: agentic-saas-fragmented-v1",
		"Eval stage: seed_smoke",
		"Retriever: eval_weighted_files_v0",
		"Token counter: approx_chars_div_4",
		"Pricing profile: none",
		"Corpus",
		"Case: resume-entitlement-sync",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestEvalCommand_JSONOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["retriever"] != "eval_weighted_files_v0" {
		t.Fatalf("retriever = %#v", got["retriever"])
	}
	if got["token_counter"] != "approx_chars_div_4" {
		t.Fatalf("token_counter = %#v", got["token_counter"])
	}
	if got["eval_stage"] != "seed_smoke" {
		t.Fatalf("eval_stage = %#v", got["eval_stage"])
	}
	if _, ok := got["corpus"].(map[string]any); !ok {
		t.Fatalf("missing corpus summary: %#v", got["corpus"])
	}
}
