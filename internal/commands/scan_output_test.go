package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestScanHuman_OutputUsesDisplayLabels(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()

	scanCmd := NewScanCmd()
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Indexed by source:") {
		t.Fatalf("missing Indexed by source header:\n%s", out)
	}
	if !strings.Contains(out, "Planning docs") {
		t.Errorf("expected Planning docs label, got:\n%s", out)
	}
	if !strings.Contains(out, "OpenSpec") {
		t.Errorf("expected OpenSpec label, got:\n%s", out)
	}
	if !strings.Contains(out, "ADRs") {
		t.Errorf("expected ADRs label, got:\n%s", out)
	}
	if strings.Contains(out, "\nFound:") {
		t.Errorf("old Found block should be removed, got:\n%s", out)
	}
}

func TestScanJSON_ConsecutiveRunsIdentical(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()

	// Establish index so JSON snapshots are both "steady state" (same New/Updated/Unchanged).
	scanWarmup := NewScanCmd()
	scanWarmup.SetOut(&bytes.Buffer{})
	if err := scanWarmup.Execute(); err != nil {
		t.Fatal(err)
	}

	first := runScanJSONBytes(t)
	second := runScanJSONBytes(t)
	if string(first) != string(second) {
		t.Errorf("consecutive --json scans differ:\n%s\n---\n%s", first, second)
	}
}

func runScanJSONBytes(t *testing.T) []byte {
	t.Helper()
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, buf.Bytes()); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return compact.Bytes()
}

func TestScan_QuietWithJSON_WritesJSONSuppressesHuman(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()
	warm := NewScanCmd()
	warm.SetOut(&bytes.Buffer{})
	if err := warm.Execute(); err != nil {
		t.Fatal(err)
	}

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--quiet"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"Found"`) {
		t.Fatalf("expected JSON with Found, got: %q", out)
	}
	if strings.Contains(out, "Indexed by source") {
		t.Fatalf("human summary should be suppressed with --quiet, got: %q", out)
	}
}
