package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	taskCheckpointJSONSchema   = "devspecs.task.checkpoint_input"
	taskCheckpointJSONMaxBytes = 256 << 10
	taskCheckpointJSONMaxItems = 256
	taskCheckpointJSONMaxID    = 128
	taskCheckpointJSONMaxPath  = 1024
	taskCheckpointJSONMaxEntry = 4096
	taskCheckpointJSONMaxProse = 16384
)

type taskCheckpointJSONInput struct {
	TaskID, Target, Stage, Decision         string
	ExpectedHeads, Supersedes               []string
	Goal, Description, Note                 string
	Resources, FilesRead, FilesEdited       []string
	TestsRead, TestsRun                     []string
	MissedFiles, NoiseFiles                 []string
	Tasks, Learnings                        []string
	NextTarget, NextDecision, DurableRecord string
	DurableArtifacts                        []string
	ReceiptEvidence                         *taskCheckpointInputReceipt
}

type taskCheckpointInputReceipt struct {
	DispatchID    string `json:"dispatch_id"`
	ReceiptID     string `json:"receipt_id"`
	RequestSHA256 string `json:"request_sha256"`
	WireSHA256    string `json:"wire_sha256"`
	ResultCommit  string `json:"result_commit,omitempty"`
}

// resource records a claim, not an authorization or a lookup reference.
func (r taskCheckpointInputReceipt) resource() string {
	data, _ := json.Marshal(r) // The receipt contains only strings.
	return "devspecs.dispatch.receipt_evidence:" + string(data)
}

var (
	taskCheckpointDispatchID = regexp.MustCompile(`^dispatch_[0-9A-HJKMNPQRSTVWXYZ]{26}$`)
	taskCheckpointReceiptID  = regexp.MustCompile(`^receipt_[0-9A-HJKMNPQRSTVWXYZ]{26}$`)
	taskCheckpointDigest     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	taskCheckpointCommit     = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	taskCheckpointDevice     = regexp.MustCompile(`^(?:[Cc][Oo][Nn]|[Pp][Rr][Nn]|[Aa][Uu][Xx]|[Nn][Uu][Ll]|[Cc][Oo][Mm][1-9\x{b9}\x{b2}\x{b3}]|[Ll][Pp][Tt][1-9\x{b9}\x{b2}\x{b3}]) *($|\.)`)
)

func parseTaskCheckpointJSON(r io.Reader) (taskCheckpointJSONInput, error) {
	var input taskCheckpointJSONInput
	raw, err := io.ReadAll(io.LimitReader(r, taskCheckpointJSONMaxBytes+1))
	if err != nil {
		return input, fmt.Errorf("read checkpoint JSON: %w", err)
	}
	if len(raw) > taskCheckpointJSONMaxBytes {
		return input, fmt.Errorf("checkpoint JSON exceeds %d bytes", taskCheckpointJSONMaxBytes)
	}
	if !utf8.Valid(raw) {
		return input, fmt.Errorf("checkpoint JSON is not valid UTF-8")
	}
	if !validTaskCheckpointJSONEscapes(raw) {
		return input, fmt.Errorf("checkpoint JSON contains an invalid Unicode escape")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var schema string
	fields := map[string]taskCheckpointJSONField{
		"schema": taskCheckpointJSONString(&schema, func(value string) error {
			if value != taskCheckpointJSONSchema {
				return fmt.Errorf("must be %q", taskCheckpointJSONSchema)
			}
			return nil
		}),
		"schema_version": func(dec *json.Decoder) error {
			token, err := dec.Token()
			if err != nil {
				return err
			}
			if token != json.Number("1") {
				return fmt.Errorf("must be the integer 1")
			}
			return nil
		},
		"task_id":           taskCheckpointJSONString(&input.TaskID, validateTaskCheckpointJSONID),
		"target":            taskCheckpointJSONString(&input.Target, validateTaskCheckpointJSONID),
		"stage":             taskCheckpointJSONString(&input.Stage, taskCheckpointJSONEnum(taskLifecycleStages...)),
		"decision":          taskCheckpointJSONString(&input.Decision, taskCheckpointJSONEnum(taskDecisions...)),
		"expected_heads":    taskCheckpointJSONList(&input.ExpectedHeads, validateTaskCheckpointJSONID, true),
		"supersedes":        taskCheckpointJSONList(&input.Supersedes, validateTaskCheckpointJSONID, true),
		"goal":              taskCheckpointJSONString(&input.Goal, taskCheckpointJSONLength(0, taskCheckpointJSONMaxProse)),
		"description":       taskCheckpointJSONString(&input.Description, taskCheckpointJSONLength(0, taskCheckpointJSONMaxProse)),
		"note":              taskCheckpointJSONString(&input.Note, taskCheckpointJSONLength(0, taskCheckpointJSONMaxProse)),
		"resources":         taskCheckpointJSONList(&input.Resources, taskCheckpointJSONLength(1, taskCheckpointJSONMaxEntry), false),
		"files_read":        taskCheckpointJSONList(&input.FilesRead, validateTaskCheckpointJSONPath, false),
		"files_edited":      taskCheckpointJSONList(&input.FilesEdited, validateTaskCheckpointJSONPath, false),
		"tests_read":        taskCheckpointJSONList(&input.TestsRead, validateTaskCheckpointJSONPath, false),
		"tests_run":         taskCheckpointJSONList(&input.TestsRun, taskCheckpointJSONLength(1, taskCheckpointJSONMaxEntry), false),
		"missed_files":      taskCheckpointJSONList(&input.MissedFiles, validateTaskCheckpointJSONPath, false),
		"noise_files":       taskCheckpointJSONList(&input.NoiseFiles, validateTaskCheckpointJSONPath, false),
		"tasks":             taskCheckpointJSONList(&input.Tasks, taskCheckpointJSONLength(1, taskCheckpointJSONMaxEntry), false),
		"learnings":         taskCheckpointJSONList(&input.Learnings, taskCheckpointJSONLength(1, taskCheckpointJSONMaxEntry), false),
		"next_target":       taskCheckpointJSONString(&input.NextTarget, taskCheckpointJSONLength(0, taskCheckpointJSONMaxID)),
		"next_decision":     taskCheckpointJSONString(&input.NextDecision, taskCheckpointJSONEnum(taskDecisions...)),
		"durable_record":    taskCheckpointJSONString(&input.DurableRecord, taskCheckpointJSONEnum("none", "recorded", "deferred")),
		"durable_artifacts": taskCheckpointJSONList(&input.DurableArtifacts, validateTaskCheckpointJSONPath, false),
		"receipt_evidence":  taskCheckpointJSONReceipt(&input.ReceiptEvidence),
	}
	err = readTaskCheckpointJSONObject(dec, fields, "schema", "schema_version", "task_id", "target", "stage", "decision", "expected_heads")
	if err != nil {
		return taskCheckpointJSONInput{}, fmt.Errorf("checkpoint JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return taskCheckpointJSONInput{}, fmt.Errorf("checkpoint JSON contains trailing data")
	}
	return input, nil
}

// encoding/json replaces unpaired surrogate escapes with U+FFFD. Reject them
// instead so evidence is not silently changed during decoding.
func validTaskCheckpointJSONEscapes(raw []byte) bool {
	for i := 0; i+1 < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if raw[i] != 'u' {
			continue
		}
		if len(raw)-i < 5 {
			return false
		}
		unit, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil || (unit >= 0xdc00 && unit <= 0xdfff) {
			return false
		}
		i += 4
		if unit >= 0xd800 && unit <= 0xdbff {
			if len(raw)-i < 7 || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

type taskCheckpointJSONField func(*json.Decoder) error

// Read only the shapes this input accepts. Tokenized names retain exact case
// and expose duplicates (including equivalent escaped spellings).
func readTaskCheckpointJSONObject(dec *json.Decoder, fields map[string]taskCheckpointJSONField, required ...string) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("expected an object")
	}
	seen := make(map[string]bool, len(fields))
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("expected an object member name")
		}
		if seen[key] {
			return fmt.Errorf("duplicate member %q", key)
		}
		read, ok := fields[key]
		if !ok {
			return fmt.Errorf("unknown object member")
		}
		if err := read(dec); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		seen[key] = true
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	for _, key := range required {
		if !seen[key] {
			return fmt.Errorf("missing required member %q", key)
		}
	}
	return nil
}

func readTaskCheckpointJSONString(dec *json.Decoder, validate func(string) error) (string, error) {
	token, err := dec.Token()
	if err != nil {
		return "", err
	}
	value, ok := token.(string)
	if !ok {
		return "", fmt.Errorf("expected a string")
	}
	return value, validate(value)
}

func taskCheckpointJSONString(dst *string, validate func(string) error) taskCheckpointJSONField {
	return func(dec *json.Decoder) error {
		value, err := readTaskCheckpointJSONString(dec, validate)
		*dst = value
		return err
	}
}

func taskCheckpointJSONList(dst *[]string, validate func(string) error, uniqueIDs bool) taskCheckpointJSONField {
	return func(dec *json.Decoder) error {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		if token != json.Delim('[') {
			return fmt.Errorf("expected an array")
		}
		values := []string{}
		var seen map[string]bool
		if uniqueIDs {
			seen = make(map[string]bool)
		}
		for dec.More() {
			if len(values) == taskCheckpointJSONMaxItems {
				return fmt.Errorf("must have at most %d items", taskCheckpointJSONMaxItems)
			}
			value, err := readTaskCheckpointJSONString(dec, validate)
			if err != nil {
				return fmt.Errorf("item %d: %w", len(values), err)
			}
			if uniqueIDs {
				key := strings.ToLower(value) // Match the event graph's ID comparison.
				if seen[key] {
					return fmt.Errorf("item %d: duplicate checkpoint ID %q", len(values), value)
				}
				seen[key] = true
			}
			values = append(values, value)
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
		*dst = values
		return nil
	}
}

func taskCheckpointJSONReceipt(dst **taskCheckpointInputReceipt) taskCheckpointJSONField {
	return func(dec *json.Decoder) error {
		receipt := new(taskCheckpointInputReceipt)
		fields := map[string]taskCheckpointJSONField{
			"dispatch_id":    taskCheckpointJSONString(&receipt.DispatchID, taskCheckpointJSONPattern(taskCheckpointDispatchID)),
			"receipt_id":     taskCheckpointJSONString(&receipt.ReceiptID, taskCheckpointJSONPattern(taskCheckpointReceiptID)),
			"request_sha256": taskCheckpointJSONString(&receipt.RequestSHA256, taskCheckpointJSONPattern(taskCheckpointDigest)),
			"wire_sha256":    taskCheckpointJSONString(&receipt.WireSHA256, taskCheckpointJSONPattern(taskCheckpointDigest)),
			"result_commit":  taskCheckpointJSONString(&receipt.ResultCommit, taskCheckpointJSONPattern(taskCheckpointCommit)),
		}
		if err := readTaskCheckpointJSONObject(dec, fields, "dispatch_id", "receipt_id", "request_sha256", "wire_sha256"); err != nil {
			return err
		}
		*dst = receipt
		return nil
	}
}

func taskCheckpointJSONLength(minimum, maximum int) func(string) error {
	return func(value string) error {
		if size := utf8.RuneCountInString(value); size < minimum || size > maximum {
			return fmt.Errorf("must contain %d to %d characters", minimum, maximum)
		}
		return nil
	}
}

func taskCheckpointJSONEnum(allowed ...string) func(string) error {
	return func(value string) error {
		if !isAllowedValue(value, allowed) {
			return fmt.Errorf("must be one of: %s", strings.Join(allowed, ", "))
		}
		return nil
	}
}

func taskCheckpointJSONPattern(pattern *regexp.Regexp) func(string) error {
	return func(value string) error {
		if !pattern.MatchString(value) {
			return fmt.Errorf("invalid format")
		}
		return nil
	}
}

func validateTaskCheckpointJSONID(value string) error {
	if err := taskCheckpointJSONLength(1, taskCheckpointJSONMaxID)(value); err != nil {
		return err
	}
	if value == "." || value == ".." {
		return fmt.Errorf("must be an ID, not a dot path")
	}
	return validateTaskID(value)
}

func validateTaskCheckpointJSONPath(value string) error {
	if err := taskCheckpointJSONLength(1, taskCheckpointJSONMaxPath)(value); err != nil {
		return err
	}
	if value == "-" || value != strings.TrimSpace(value) || strings.ContainsAny(value, `\:#<>"|?*`) ||
		strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("must be a portable repository-relative path")
	}
	for _, part := range strings.Split(value, "/") {
		// Trailing dots/spaces also exclude Windows aliases for dot segments.
		if part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || taskCheckpointDevice.MatchString(part) {
			return fmt.Errorf("must be a portable repository-relative path")
		}
	}
	return nil
}
