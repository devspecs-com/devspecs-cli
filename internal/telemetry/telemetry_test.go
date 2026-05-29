package telemetry

import "testing"

func TestSanitizePropertiesKeepsOnlyAllowedCoarseFields(t *testing.T) {
	props := sanitizeProperties(map[string]any{
		"command":             "scan",
		"success":             true,
		"query":               "do not send me",
		"repo_path":           "/private/repo",
		"query_length_bucket": "11-50",
	})

	if props["command"] != "scan" || props["success"] != true || props["query_length_bucket"] != "11-50" {
		t.Fatalf("expected allowed properties to remain: %#v", props)
	}
	if _, ok := props["query"]; ok {
		t.Fatalf("raw query should be dropped: %#v", props)
	}
	if _, ok := props["repo_path"]; ok {
		t.Fatalf("repo path should be dropped: %#v", props)
	}
}

func TestBucketsAreCoarse(t *testing.T) {
	tests := map[int]string{
		0:   "0",
		1:   "1-10",
		10:  "1-10",
		11:  "11-50",
		50:  "11-50",
		51:  "51-100",
		100: "51-100",
		101: "101-500",
		501: "501+",
	}
	for n, want := range tests {
		if got := CountBucket(n); got != want {
			t.Fatalf("CountBucket(%d) = %q, want %q", n, got, want)
		}
	}
}
