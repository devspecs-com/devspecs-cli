package orchestration

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// M1 draft 2 snapshot. RepoPath expresses its lookaheads as separate "not"
// patterns so the existing validator can use Go's regular expression engine.
//
//go:embed http_dispatch_v2.schema.json
var httpDispatchV2Schema []byte

var httpDispatchSchemas = sync.OnceValues(func() (map[string]*jsonschema.Schema, error) {
	var document any
	if err := json.Unmarshal(httpDispatchV2Schema, &document); err != nil {
		return nil, err
	}
	const resource = "urn:devspecs:http-dispatch-v2"
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, err
	}
	schemas := make(map[string]*jsonschema.Schema)
	for _, name := range []string{"Capabilities", "Request", "PortableApply", "Handle", "Status", "Receipt", "Error"} {
		schema, err := compiler.Compile(resource + "#/$defs/" + name)
		if err != nil {
			return nil, err
		}
		schemas[name] = schema
	}
	return schemas, nil
})

func decodeHTTPDispatch(raw []byte, name string, out any) bool {
	if !utf8.Valid(raw) {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	value, err := readHTTPDispatchValue(dec, 0)
	if err != nil {
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		return false
	}
	schemas, err := httpDispatchSchemas()
	return err == nil && schemas[name].Validate(value) == nil && json.Unmarshal(raw, out) == nil
}

// Validate exact member names and reject duplicates before decoding into a Go struct.
func readHTTPDispatchValue(dec *json.Decoder, depth int) (any, error) {
	if depth > 16 {
		return nil, errHTTPDispatchResponse
	}
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := make(map[string]any)
		for dec.More() {
			token, err := dec.Token()
			key, ok := token.(string)
			if err != nil || !ok {
				return nil, errHTTPDispatchResponse
			}
			if _, exists := object[key]; exists {
				return nil, errHTTPDispatchResponse
			}
			value, err := readHTTPDispatchValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		_, err := dec.Token()
		return object, err
	case json.Delim('['):
		array := []any{}
		for dec.More() {
			value, err := readHTTPDispatchValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err := dec.Token()
		return array, err
	default:
		// All v2 numeric fields are bounded integers. Reject extreme exponents
		// before they reach the validator's arbitrary-precision arithmetic.
		if number, ok := token.(json.Number); ok {
			if _, err := number.Int64(); err != nil {
				return nil, errHTTPDispatchResponse
			}
		}
		return token, nil
	}
}

var httpDispatchID = regexp.MustCompile(`^dispatch_[0-9A-HJKMNPQRSTVWXYZ]{26}$`)

type httpDispatchCapabilities struct {
	Provider                 string `json:"provider"`
	Ready                    bool   `json:"ready"`
	Isolation                bool   `json:"isolation"`
	Report                   bool   `json:"report"`
	Cancellation             bool   `json:"cancellation"`
	IdempotencyRetentionSecs int    `json:"idempotency_retention_seconds"`
	VerificationProfiles     []struct {
		Key      string `json:"key"`
		Strength string `json:"strength"`
	} `json:"verification_profiles"`
	Limits struct {
		MaxRequestBytes   int   `json:"max_request_bytes"`
		MaxInlineBytes    int64 `json:"max_inline_bytes"`
		MaxArtifactBytes  int64 `json:"max_artifact_bytes"`
		MaxTimeoutSeconds int   `json:"max_timeout_seconds"`
		MaxAttempts       int   `json:"max_attempts"`
	} `json:"limits"`

	transport *httpDispatchTransport
}

type httpDispatchRepository struct {
	LogicalID    string `json:"logical_id"`
	SourceCommit string `json:"source_commit"`
}

type httpDispatchCaller struct {
	PrincipalID  string `json:"principal_id"`
	CredentialID string `json:"credential_id"`
}

type httpDispatchRequest struct {
	DispatchID string             `json:"dispatch_id"`
	Caller     httpDispatchCaller `json:"caller"`
	Target     struct {
		Slice string `json:"slice"`
	} `json:"target"`
	Repository httpDispatchRepository `json:"repository"`
	Input      struct {
		ApplyPayload     string                 `json:"apply_payload"`
		ApplySHA256      string                 `json:"apply_sha256"`
		PromptSHA256     string                 `json:"execution_prompt_sha256"`
		ContextArtifacts []httpDispatchArtifact `json:"context_artifacts"`
	} `json:"input"`
	Requirements struct {
		TimeoutSeconds int    `json:"timeout_seconds"`
		MaxAttempts    int    `json:"max_attempts"`
		Cancellation   string `json:"cancellation"`
		Report         struct {
			Required bool `json:"required"`
		} `json:"report"`
		Verification struct {
			Required bool   `json:"required"`
			Profile  string `json:"profile"`
			Strength string `json:"strength"`
		} `json:"verification"`
	} `json:"requirements"`
}

func (r httpDispatchRequest) validateInput() (inlineBytes, artifactBytes int64, valid bool) {
	apply := []byte(r.Input.ApplyPayload)
	var projection struct {
		Target string `json:"target"`
		Prompt string `json:"prompt"`
	}
	if httpDispatchDigest(apply) != r.Input.ApplySHA256 || !decodeHTTPDispatch(apply, "PortableApply", &projection) ||
		projection.Target != r.Target.Slice || httpDispatchDigest([]byte(projection.Prompt)) != r.Input.PromptSHA256 {
		return 0, 0, false
	}
	inlineBytes = int64(len(apply))
	for _, artifact := range r.Input.ContextArtifacts {
		if artifact.Kind == "https" {
			artifactBytes += artifact.Size
		} else if artifact.validInline() {
			inlineBytes += artifact.Size
		} else {
			return 0, 0, false
		}
	}
	return inlineBytes, artifactBytes, true
}

func (c *httpDispatchCapabilities) allows(r httpDispatchRequest, requestBytes int, inlineBytes, artifactBytes int64) bool {
	req, limits := r.Requirements, c.Limits
	if !c.Ready || !c.Isolation || (req.Report.Required && !c.Report) || (req.Cancellation == "required" && !c.Cancellation) ||
		requestBytes > limits.MaxRequestBytes || inlineBytes > limits.MaxInlineBytes || artifactBytes > limits.MaxArtifactBytes ||
		req.TimeoutSeconds > limits.MaxTimeoutSeconds || req.MaxAttempts > limits.MaxAttempts {
		return false
	}
	if !req.Verification.Required {
		return true
	}
	for _, profile := range c.VerificationProfiles {
		if profile.Key == req.Verification.Profile && profile.Strength == req.Verification.Strength {
			return true
		}
	}
	return false
}

type httpDispatchCorrelation struct {
	DispatchID    string `json:"dispatch_id"`
	RequestSHA256 string `json:"request_sha256"`
	Provider      struct {
		Name  string `json:"name"`
		RunID string `json:"run_id"`
	} `json:"provider"`
}

type httpDispatchHandle struct {
	httpDispatchCorrelation
	AcceptedAt time.Time `json:"accepted_at"`

	transport   *httpDispatchTransport
	repository  httpDispatchRepository
	caller      httpDispatchCaller
	maxAttempts int
}

type httpDispatchStatus struct {
	httpDispatchCorrelation
	State           string          `json:"state"`
	CancelRequested bool            `json:"cancel_requested"`
	ObservedAt      time.Time       `json:"observed_at"`
	Usage           json.RawMessage `json:"usage,omitempty"`
}

type httpDispatchReceipt struct {
	httpDispatchCorrelation
	Caller     httpDispatchCaller     `json:"caller"`
	Repository httpDispatchRepository `json:"repository"`
	Execution  struct {
		Attempts  int        `json:"attempts"`
		StartedAt *time.Time `json:"started_at"`
		EndedAt   time.Time  `json:"ended_at"`
	} `json:"execution"`
	Evidence struct {
		Manifest *httpDispatchArtifact `json:"paths_manifest"`
	} `json:"evidence"`
	Report struct {
		Artifact *httpDispatchArtifact `json:"artifact"`
	} `json:"report"`
	Verification struct {
		TestOutput *httpDispatchArtifact `json:"test_output"`
	} `json:"verification"`
}

type httpDispatchArtifact struct {
	Kind    string `json:"kind"`
	Size    int64  `json:"size_bytes"`
	SHA256  string `json:"sha256"`
	Content string `json:"content_base64"`
}

func (a *httpDispatchArtifact) validInline() bool {
	if a == nil || a.Kind != "inline" {
		return true
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(a.Content)
	return err == nil && !strings.ContainsAny(a.Content, "\r\n") && int64(len(raw)) == a.Size && httpDispatchDigest(raw) == a.SHA256
}

func httpDispatchDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func terminalHTTPDispatchState(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}
