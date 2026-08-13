package devhome

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	MarkerFilename    = "development-home.json"
	markerSchema      = 1
	managedBy         = "devspecs-cli"
	defaultRootName   = ".devspecs-dev"
	stableRootName    = ".devspecs"
	telemetryDisabled = "disabled"
)

var channelKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type Scope string

const (
	ScopeChannel  Scope = "channel"
	ScopeWorktree Scope = "worktree"
)

type Options struct {
	Channel    string
	Worktree   bool
	SourceRoot string
	DevRoot    string
	UserHome   string
}

type Selection struct {
	Scope            Scope  `json:"scope"`
	Key              string `json:"key"`
	Home             string `json:"home"`
	SourceRoot       string `json:"source_root"`
	SourceRootHash   string `json:"source_root_hash"`
	TelemetryDefault string `json:"telemetry_default"`
}

type Marker struct {
	SchemaVersion    int    `json:"schema_version"`
	ManagedBy        string `json:"managed_by"`
	Scope            Scope  `json:"scope"`
	Key              string `json:"key"`
	SourceRoot       string `json:"source_root"`
	SourceRootHash   string `json:"source_root_hash"`
	CreatedAt        string `json:"created_at"`
	LastUsedAt       string `json:"last_used_at"`
	TelemetryDefault string `json:"telemetry_default"`
}

func Resolve(opts Options) (Selection, error) {
	if opts.Channel != "" && opts.Worktree {
		return Selection{}, fmt.Errorf("--channel and --worktree are mutually exclusive")
	}

	sourceRoot, err := canonicalPath(opts.SourceRoot)
	if err != nil {
		return Selection{}, fmt.Errorf("resolve source root: %w", err)
	}
	userHome, err := resolveUserHome(opts.UserHome)
	if err != nil {
		return Selection{}, err
	}
	devRoot, err := resolveDevRoot(opts.DevRoot, userHome)
	if err != nil {
		return Selection{}, err
	}
	stableRoot := filepath.Join(userHome, stableRootName)
	insideStable, err := pathIsSameOrDescendant(devRoot, stableRoot)
	if err != nil {
		return Selection{}, fmt.Errorf("compare development and stable homes: %w", err)
	}
	if insideStable {
		return Selection{}, fmt.Errorf("development home root must not be the stable home or one of its descendants: %s", devRoot)
	}

	sourceHash := sourceRootHash(sourceRoot, runtime.GOOS == "windows")
	selection := Selection{
		SourceRoot:       sourceRoot,
		SourceRootHash:   sourceHash,
		TelemetryDefault: telemetryDisabled,
	}
	if opts.Channel != "" {
		key, err := normalizeChannelKey(opts.Channel)
		if err != nil {
			return Selection{}, err
		}
		selection.Scope = ScopeChannel
		selection.Key = key
		selection.Home = filepath.Join(devRoot, "channels", key)
		return selection, nil
	}

	selection.Scope = ScopeWorktree
	selection.Key = sourceSlug(sourceRoot) + "-" + sourceHash
	selection.Home = filepath.Join(devRoot, "worktrees", selection.Key)
	return selection, nil
}

func Prepare(selection Selection, now time.Time) (Marker, error) {
	if err := validateSelection(selection); err != nil {
		return Marker{}, err
	}
	if err := prepareDirectory(selection.Home); err != nil {
		return Marker{}, err
	}

	markerPath := filepath.Join(selection.Home, MarkerFilename)
	createdAt := now.UTC().Format(time.RFC3339)
	if existing, err := readMarker(markerPath); err == nil {
		if err := validateMarker(existing, selection); err != nil {
			return Marker{}, err
		}
		createdAt = existing.CreatedAt
	} else if !os.IsNotExist(err) {
		return Marker{}, err
	}

	marker := Marker{
		SchemaVersion:    markerSchema,
		ManagedBy:        managedBy,
		Scope:            selection.Scope,
		Key:              selection.Key,
		SourceRoot:       selection.SourceRoot,
		SourceRootHash:   selection.SourceRootHash,
		CreatedAt:        createdAt,
		LastUsedAt:       now.UTC().Format(time.RFC3339),
		TelemetryDefault: telemetryDisabled,
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return Marker{}, fmt.Errorf("encode development home marker: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(markerPath, data, 0o600); err != nil {
		return Marker{}, fmt.Errorf("write development home marker: %w", err)
	}
	return marker, nil
}

func ChildEnvironment(base []string, home string) []string {
	hasTelemetryMode := environmentValue(base, "DEVSPECS_TELEMETRY") != "" || environmentValue(base, "DS_TELEMETRY") != ""
	out := make([]string, 0, len(base)+2)
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok && environmentKeyEqual(key, "DEVSPECS_HOME") {
			continue
		}
		if ok && !hasTelemetryMode && (environmentKeyEqual(key, "DEVSPECS_TELEMETRY") || environmentKeyEqual(key, "DS_TELEMETRY")) {
			continue
		}
		out = append(out, item)
	}
	out = append(out, "DEVSPECS_HOME="+home)
	if !hasTelemetryMode {
		out = append(out, "DEVSPECS_TELEMETRY=0")
	}
	return out
}

func normalizeChannelKey(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !channelKeyPattern.MatchString(value) {
		return "", fmt.Errorf("channel key must match %s", channelKeyPattern.String())
	}
	if value == "stable" || value == "default" {
		return "", fmt.Errorf("channel key %q is reserved", value)
	}
	return value, nil
}

func resolveUserHome(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		var err error
		value, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
	}
	home, err := canonicalPath(value)
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return home, nil
}

func resolveDevRoot(value, userHome string) (string, error) {
	if strings.TrimSpace(value) == "" {
		value = filepath.Join(userHome, defaultRootName)
	}
	root, err := canonicalPath(value)
	if err != nil {
		return "", fmt.Errorf("resolve development home root: %w", err)
	}
	return root, nil
}

func canonicalPath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		var err error
		value, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = filepath.Clean(resolved)
	}
	return abs, nil
}

func sourceRootHash(root string, windows bool) string {
	normalized := filepath.ToSlash(filepath.Clean(root))
	if windows {
		normalized = strings.ToLower(normalized)
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:12]
}

func sourceSlug(root string) string {
	value := strings.ToLower(filepath.Base(filepath.Clean(root)))
	var builder strings.Builder
	separator := false
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			separator = false
		case !separator:
			builder.WriteByte('-')
			separator = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "source"
	}
	return slug
}

func pathIsSameOrDescendant(path, parent string) (bool, error) {
	relative, err := filepath.Rel(parent, path)
	if err != nil {
		return false, err
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}

func validateSelection(selection Selection) error {
	if selection.Scope != ScopeChannel && selection.Scope != ScopeWorktree {
		return fmt.Errorf("unsupported development home scope %q", selection.Scope)
	}
	if selection.Key == "" || selection.Home == "" || selection.SourceRoot == "" || selection.SourceRootHash == "" {
		return fmt.Errorf("development home selection is incomplete")
	}
	return nil
}

func prepareDirectory(path string) error {
	info, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create development home: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("inspect development home: %w", err)
	case !info.IsDir():
		return fmt.Errorf("development home is not a directory: %s", path)
	}

	markerPath := filepath.Join(path, MarkerFilename)
	if _, err := os.Stat(markerPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect development home marker: %w", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("inspect development home contents: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("refusing to adopt non-empty unmarked development home: %s", path)
	}
	return nil
}

func readMarker(path string) (Marker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Marker{}, err
	}
	var marker Marker
	if err := json.Unmarshal(data, &marker); err != nil {
		return Marker{}, fmt.Errorf("parse development home marker: %w", err)
	}
	return marker, nil
}

func validateMarker(marker Marker, selection Selection) error {
	if marker.SchemaVersion != markerSchema || marker.ManagedBy != managedBy {
		return fmt.Errorf("development home marker is not supported")
	}
	if marker.Scope != selection.Scope || marker.Key != selection.Key {
		return fmt.Errorf("development home marker does not match selected scope")
	}
	if selection.Scope == ScopeWorktree && marker.SourceRootHash != selection.SourceRootHash {
		return fmt.Errorf("development home marker does not match selected worktree")
	}
	if marker.CreatedAt == "" {
		return fmt.Errorf("development home marker is missing created_at")
	}
	return nil
}

func environmentValue(environment []string, key string) string {
	value := ""
	for _, item := range environment {
		itemKey, itemValue, ok := strings.Cut(item, "=")
		if ok && environmentKeyEqual(itemKey, key) {
			value = itemValue
		}
	}
	return value
}

func environmentKeyEqual(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
