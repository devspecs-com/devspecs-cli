// Package telemetry sends coarse, privacy-preserving CLI usage events.
package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/version"
)

const (
	defaultEndpoint = "https://devspecs.com/api/telemetry"
	defaultTimeout  = 750 * time.Millisecond
)

var sessionID = randomID("sess")

// Event is the wire shape accepted by the website telemetry endpoint.
type Event struct {
	Event       string         `json:"event"`
	AnonymousID string         `json:"anonymous_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	CLIVersion  string         `json:"cli_version,omitempty"`
	OS          string         `json:"os,omitempty"`
	Arch        string         `json:"arch,omitempty"`
	OccurredAt  string         `json:"occurred_at,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
}

// Record sends one best-effort event. It never returns an error to callers.
func Record(ctx context.Context, name string, properties map[string]any) {
	cfg := loadConfig()
	if !cfg.enabled {
		return
	}

	event := Event{
		Event:       name,
		AnonymousID: anonymousID(),
		SessionID:   sessionID,
		CLIVersion:  version.Version,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		OccurredAt:  time.Now().UTC().Format(time.RFC3339),
		Properties:  sanitizeProperties(properties),
	}

	if cfg.debug {
		enc, _ := json.Marshal(event)
		fmt.Fprintf(os.Stderr, "devspecs telemetry debug: %s\n", enc)
		return
	}

	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("user-agent", "devspecs-cli/"+version.Version)

	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

// RecordCommand sends a coarse command completion event.
func RecordCommand(command string, success bool, duration time.Duration, properties map[string]any) {
	props := map[string]any{
		"command":         command,
		"success":         success,
		"duration_bucket": durationBucket(duration),
	}
	for k, v := range properties {
		props[k] = v
	}
	Record(context.Background(), command+"_completed", props)
}

type runtimeConfig struct {
	enabled  bool
	debug    bool
	endpoint string
}

func loadConfig() runtimeConfig {
	mode := firstEnv("DEVSPECS_TELEMETRY", "DS_TELEMETRY")
	if disabledMode(mode) || os.Getenv("CI") != "" || runningTestBinary() {
		return runtimeConfig{}
	}
	endpoint := firstEnv("DEVSPECS_TELEMETRY_URL", "DS_TELEMETRY_URL")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return runtimeConfig{
		enabled:  true,
		debug:    strings.EqualFold(mode, "debug"),
		endpoint: endpoint,
	}
}

func disabledMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "0", "false", "off", "no", "disabled":
		return true
	default:
		return false
	}
}

func runningTestBinary() bool {
	base := strings.ToLower(filepath.Base(os.Args[0]))
	return strings.HasSuffix(base, ".test") || strings.Contains(base, ".test.")
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func anonymousID() string {
	home, err := config.HomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, "telemetry.json")
	if data, err := os.ReadFile(path); err == nil {
		var stored struct {
			AnonymousID string `json:"anonymous_id"`
		}
		if json.Unmarshal(data, &stored) == nil && stored.AnonymousID != "" {
			return stored.AnonymousID
		}
	}

	id := randomID("anon")
	if id == "" {
		return ""
	}
	_ = os.MkdirAll(home, 0o755)
	data, _ := json.MarshalIndent(struct {
		AnonymousID string `json:"anonymous_id"`
	}{AnonymousID: id}, "", "  ")
	_ = os.WriteFile(path, data, 0o600)
	return id
}

func randomID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func sanitizeProperties(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if !allowedProperty(k) {
			continue
		}
		switch value := v.(type) {
		case string:
			if len(value) > 80 {
				value = value[:80]
			}
			out[k] = value
		case bool:
			out[k] = value
		case int:
			out[k] = value
		case int64:
			out[k] = value
		case float64:
			out[k] = value
		default:
			out[k] = fmt.Sprint(value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func allowedProperty(key string) bool {
	switch key {
	case "command", "success", "duration_bucket", "error_class",
		"install_method", "install_os", "install_arch", "install_version",
		"force", "hooks", "no_detect", "interactive",
		"include_tests", "include_code_comments", "if_changed", "rebuild", "json", "quiet",
		"artifact_count_bucket", "new_count_bucket", "updated_count_bucket", "unchanged_count_bucket", "source_count_bucket", "found_any",
		"query_length_bucket", "result_count_bucket", "focused":
		return true
	default:
		return false
	}
}

// CountBucket coarsens counts before telemetry.
func CountBucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n <= 10:
		return "1-10"
	case n <= 50:
		return "11-50"
	case n <= 100:
		return "51-100"
	case n <= 500:
		return "101-500"
	default:
		return "501+"
	}
}

// QueryLengthBucket coarsens query length without sending query text.
func QueryLengthBucket(query string) string {
	return CountBucket(len(strings.TrimSpace(query)))
}

func durationBucket(d time.Duration) string {
	ms := d.Milliseconds()
	switch {
	case ms < 100:
		return "<100ms"
	case ms < 500:
		return "100-499ms"
	case ms < 1000:
		return "500-999ms"
	case ms < 5000:
		return "1-4s"
	case ms < 30000:
		return "5-29s"
	case ms < 120000:
		return "30-119s"
	default:
		return strconv.FormatInt(ms/1000, 10) + "s+"
	}
}
