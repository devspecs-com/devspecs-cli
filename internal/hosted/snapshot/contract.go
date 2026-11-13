package snapshot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	Schema          = "devspecs.hosted.snapshot"
	MaxBodyBytes    = 8 << 20
	MaxDecodedBytes = 4 << 20
	MaxFileBytes    = 512 << 10
	MaxFiles        = 1024
	MaxRecords      = 512
	MaxPathBytes    = 1024
	MaxIDBytes      = 128
	MaxPathDepth    = 16
	MaxJSONDepth    = 64
	MaxRoots        = 32
	MaxParents      = 8
)

var (
	ErrInvalid     = errors.New("invalid snapshot")
	ErrUnsupported = errors.New("unsupported record schema")
	ErrProvenance  = errors.New("unverified provenance")
	ErrConflict    = errors.New("record conflict")
	ErrTooLarge    = errors.New("snapshot exceeds bounds")
	objectID       = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	digest         = regexp.MustCompile(`^[a-f0-9]{64}$`)
	identifier     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
	policyID       = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
)

// Field declaration order is part of the version-1 canonical JSON contract.
type Snapshot struct {
	Schema        string   `json:"schema"`
	SchemaVersion int      `json:"schema_version"`
	PolicyID      string   `json:"policy_id"`
	Commit        string   `json:"commit"`
	Ref           string   `json:"ref"`
	Records       []Record `json:"records"`
}

type Record struct {
	Kind          string `json:"kind"`
	OwnerKind     string `json:"owner_kind"`
	OwnerID       string `json:"owner_id"`
	RecordID      string `json:"record_id"`
	SchemaVersion int    `json:"schema_version"`
	Files         []File `json:"files"`
}

type File struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Content []byte `json:"content_base64"`
}

type Key struct {
	Kind      string
	OwnerKind string
	OwnerID   string
	RecordID  string
}

func (r Record) Key() Key { return Key{r.Kind, r.OwnerKind, r.OwnerID, r.RecordID} }

type Root struct {
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	ArtifactRoot string `json:"artifact_root,omitempty"`
}

// Policy is the public collection scope, not a claim of whole-repository coverage.
// Repository paths and approved source identity never enter this response.
type Policy struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	PolicyID      string `json:"policy_id"`
	Roots         []Root `json:"roots"`
}

func (p Policy) Validate() error {
	if p.Schema != "devspecs.hosted.snapshot.policy" || p.SchemaVersion != 1 || !policyID.MatchString(p.PolicyID) || len(p.Roots) == 0 || len(p.Roots) > MaxRoots {
		return ErrInvalid
	}
	labels := make(map[string]bool)
	paths := make(map[string]bool)
	for _, root := range p.Roots {
		if !ValidID(root.Label) || !ValidPath(root.Label) || strings.Contains(root.Label, "/") || !ValidPath(root.Path) || labels[root.Label] || paths[root.Path] {
			return ErrInvalid
		}
		switch root.Kind {
		case "task_parent", "workspace_manifest", "adr", "rfc", "prd":
		default:
			return ErrUnsupported
		}
		labels[root.Label], paths[root.Path] = true, true
		if root.ArtifactRoot != "" && (root.Kind != "workspace_manifest" || !ValidPath(root.ArtifactRoot)) {
			return ErrInvalid
		}
	}
	return nil
}

func ValidID(s string) bool     { return len(s) <= MaxIDBytes && identifier.MatchString(s) }
func ValidCommit(s string) bool { return objectID.MatchString(s) }
func ValidUUID(s string) bool   { return policyID.MatchString(s) }

func ValidPath(s string) bool {
	if len(s) == 0 || len(s) > MaxPathBytes || !utf8.ValidString(s) || path.Clean(s) != s || strings.HasPrefix(s, "/") || strings.ContainsAny(s, "\\:\x00\r\n\t") {
		return false
	}
	parts := strings.Split(s, "/")
	if len(parts) > MaxPathDepth {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
		for _, ch := range part {
			if ch < 32 || ch == 127 {
				return false
			}
		}
	}
	return true
}

func ValidRef(s string) bool {
	if len(s) > 256 || !strings.HasPrefix(s, "refs/heads/") || strings.Contains(s, "..") || strings.Contains(s, "@{") || strings.ContainsAny(s, " ~^:?*[\\") || !ValidPath(s) {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return len(strings.TrimPrefix(s, "refs/heads/")) > 0
}

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s Snapshot) Validate() error {
	if s.Schema != Schema || s.SchemaVersion != 1 || !policyID.MatchString(s.PolicyID) || !ValidCommit(s.Commit) || !ValidRef(s.Ref) || s.Records == nil {
		return ErrInvalid
	}
	if len(s.Records) > MaxRecords {
		return ErrTooLarge
	}
	keys := make(map[Key]bool)
	paths := make(map[string]bool)
	count, total := 0, 0
	for _, record := range s.Records {
		if keys[record.Key()] || !ValidID(record.OwnerID) || !ValidID(record.OwnerKind) || record.Files == nil || len(record.Files) == 0 {
			return ErrInvalid
		}
		keys[record.Key()] = true
		switch record.Kind {
		case "task", "thread", "change", "workspace":
			if record.SchemaVersion != 1 {
				return ErrUnsupported
			}
		case "checkpoint":
			if record.SchemaVersion != 3 {
				return ErrUnsupported
			}
		case "adr", "rfc", "prd":
			if record.SchemaVersion != 1 || !ValidPath(record.RecordID) {
				return ErrUnsupported
			}
		default:
			return ErrUnsupported
		}
		if record.Kind != "adr" && record.Kind != "rfc" && record.Kind != "prd" && !ValidID(record.RecordID) {
			return ErrInvalid
		}
		for _, file := range record.Files {
			count++
			total += len(file.Content)
			if len(file.Content) > MaxFileBytes || count > MaxFiles || total > MaxDecodedBytes {
				return ErrTooLarge
			}
			if file.Content == nil || !ValidPath(file.Path) || paths[file.Path] || !digest.MatchString(file.SHA256) || file.SHA256 != Hash(file.Content) {
				return ErrInvalid
			}
			paths[file.Path] = true
		}
		if record.Kind == "checkpoint" {
			if _, err := CheckpointDigest(record); err != nil {
				return err
			}
		}
	}
	return nil
}

// Canonical does not mutate caller-owned slices or depend on JSON input order.
func Canonical(s Snapshot) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	s.Records = append([]Record{}, s.Records...)
	for i := range s.Records {
		s.Records[i].Files = append([]File{}, s.Records[i].Files...)
		sort.Slice(s.Records[i].Files, func(a, b int) bool { return s.Records[i].Files[a].Path < s.Records[i].Files[b].Path })
	}
	sort.Slice(s.Records, func(i, j int) bool { return keyLess(s.Records[i].Key(), s.Records[j].Key()) })
	data, err := json.Marshal(s)
	if err != nil || len(data) > MaxBodyBytes {
		return nil, ErrInvalid
	}
	return data, nil
}

func keyLess(a, b Key) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.OwnerKind != b.OwnerKind {
		return a.OwnerKind < b.OwnerKind
	}
	if a.OwnerID != b.OwnerID {
		return a.OwnerID < b.OwnerID
	}
	return a.RecordID < b.RecordID
}

// The two component roles, not their locations, bind checkpoint immutability.
func CheckpointDigest(r Record) (string, error) {
	if r.Kind != "checkpoint" || len(r.Files) != 2 {
		return "", ErrInvalid
	}
	var event, markdown *File
	for i := range r.Files {
		switch path.Ext(r.Files[i].Path) {
		case ".json":
			if event != nil {
				return "", ErrInvalid
			}
			event = &r.Files[i]
		case ".md":
			if markdown != nil {
				return "", ErrInvalid
			}
			markdown = &r.Files[i]
		default:
			return "", ErrInvalid
		}
	}
	if event == nil || markdown == nil {
		return "", ErrInvalid
	}
	data, err := json.Marshal(struct {
		Event    []byte `json:"event"`
		Markdown []byte `json:"markdown"`
	}{event.Content, markdown.Content})
	if err != nil {
		return "", ErrInvalid
	}
	return Hash(data), nil
}

func Decode(r io.Reader) (Snapshot, error) {
	var s Snapshot
	data, err := io.ReadAll(io.LimitReader(r, MaxBodyBytes+1))
	if len(data) > MaxBodyBytes {
		return s, ErrTooLarge
	}
	if err != nil {
		return s, errors.Join(ErrInvalid, err)
	}
	if !utf8.Valid(data) || CheckJSON(data) != nil {
		return s, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&s) != nil {
		return Snapshot{}, ErrInvalid
	}
	if err := s.Validate(); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

// CheckJSON uses the structured token API to reject duplicate keys and bound depth.
func CheckJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	nodes := 0
	if err := jsonValue(decoder, 0, &nodes); err != nil {
		return ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

func jsonValue(d *json.Decoder, depth int, nodes *int) error {
	*nodes++
	if depth > MaxJSONDepth || *nodes > 50000 {
		return ErrInvalid
	}
	token, err := d.Token()
	if err != nil {
		return ErrInvalid
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	if delim != '{' && delim != '[' {
		return ErrInvalid
	}
	seen := make(map[string]bool)
	for d.More() {
		if delim == '{' {
			keyToken, err := d.Token()
			if err != nil {
				return ErrInvalid
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return ErrInvalid
			}
			seen[key] = true
		}
		if err := jsonValue(d, depth+1, nodes); err != nil {
			return err
		}
	}
	end, err := d.Token()
	if err != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return ErrInvalid
	}
	return nil
}
