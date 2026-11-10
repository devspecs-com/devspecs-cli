package orchestration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	httpDispatchMaxInline    = 1 << 20 // Decoded artifacts plus the portable apply JSON.
	httpDispatchMaxArtifacts = 64
)

// A trusted caller confirms this exact URL and commit as a published source.
// The URL must be unsigned and approved for disclosure. The builder checks local
// HEAD and status only; it never proves remote reachability.
type httpDispatchConfirmedSource struct {
	URL    string
	Commit string
}

type httpDispatchInlineInput struct {
	ID, MediaType, SHA256 string
	Content               []byte // nil means unavailable, not an empty artifact.
}

type httpDispatchAgentOptions struct {
	AgentProvider string `json:"agent_provider"`
	Model         string `json:"model,omitempty"`
	Effort        string `json:"effort,omitempty"`
}

type httpDispatchVerificationProfile struct {
	Profile  string `json:"profile,omitempty"`
	Strength string `json:"strength,omitempty"`
}

// Only caller-reviewed portable input belongs here. In particular, the prompt
// must already contain the intended execution instructions. The builder cannot
// determine whether arbitrary prompt or artifact content is safe to disclose.
type httpDispatchBuildSpec struct {
	DispatchID           string
	CreatedAt            time.Time
	Target               Target
	Source               *httpDispatchConfirmedSource
	ApprovedPrompt       string
	Artifacts            []httpDispatchInlineInput
	TimeoutSeconds       int
	MaxAttempts          int
	ReportRequired       bool
	CancellationRequired bool
	Verification         *httpDispatchVerificationProfile
	Options              httpDispatchAgentOptions
}

type httpDispatchPortableApply struct {
	Target string `json:"target"`
	Prompt string `json:"prompt"`
}

type httpDispatchPortableArtifact struct {
	ID        string `json:"artifact_id"`
	MediaType string `json:"media_type"`
	httpDispatchArtifact
}

type httpDispatchPortableRequest struct {
	Schema        string    `json:"schema"`
	SchemaVersion int       `json:"schema_version"`
	DispatchID    string    `json:"dispatch_id"`
	CreatedAt     time.Time `json:"created_at"`
	Target        Target    `json:"target"`
	Repository    struct {
		httpDispatchRepository
		GitURL string `json:"git_url"`
	} `json:"repository"`
	Input struct {
		ApplyPayload     string                         `json:"apply_payload"`
		ApplySHA256      string                         `json:"apply_sha256"`
		PromptSHA256     string                         `json:"execution_prompt_sha256"`
		ContextArtifacts []httpDispatchPortableArtifact `json:"context_artifacts"`
		ExecutionPolicy  ExecutionPolicy                `json:"execution_policy"`
	} `json:"input"`
	Requirements struct {
		TimeoutSeconds int    `json:"timeout_seconds"`
		MaxAttempts    int    `json:"max_attempts"`
		Isolation      string `json:"isolation"`
		Cancellation   string `json:"cancellation"`
		Report         struct {
			Required bool `json:"required"`
		} `json:"report"`
		Verification struct {
			Required bool `json:"required"`
			httpDispatchVerificationProfile
		} `json:"verification"`
	} `json:"requirements"`
	Options httpDispatchAgentOptions `json:"options"`
}

var (
	httpDispatchCommit       = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	httpDispatchPortableName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// The returned bytes are the frozen POST entity, not a value to re-marshal.
func buildHTTPDispatchRequest(ctx context.Context, runner CommandRunner, checkout string, spec httpDispatchBuildSpec) ([]byte, error) {
	if spec.Source == nil || !validHTTPDispatchSource(*spec.Source) {
		return nil, fmt.Errorf("%w: an exact HTTPS source URL and commit must be confirmed", errHTTPDispatchRequest)
	}
	if len(spec.ApprovedPrompt) > httpDispatchMaxInline || len(spec.Artifacts) > httpDispatchMaxArtifacts {
		return nil, errHTTPDispatchLimit
	}
	if !spec.validMetadata() || !utf8.ValidString(spec.ApprovedPrompt) || strings.TrimSpace(spec.ApprovedPrompt) == "" {
		return nil, errHTTPDispatchRequest
	}

	// Bound the raw inputs first, then account for escaping in the apply JSON
	// before base64 encoding any artifacts.
	remaining := httpDispatchMaxInline - len(spec.ApprovedPrompt)
	for _, artifact := range spec.Artifacts {
		if len(artifact.Content) > remaining {
			return nil, errHTTPDispatchLimit
		}
		remaining -= len(artifact.Content)
		if artifact.Content == nil || !validHTTPDispatchName(artifact.ID, 256) ||
			!validHTTPDispatchText(artifact.MediaType, 128) || len(artifact.SHA256) != len("sha256:")+64 {
			return nil, errHTTPDispatchRequest
		}
		if media, _, err := mime.ParseMediaType(artifact.MediaType); err != nil || !strings.Contains(media, "/") {
			return nil, errHTTPDispatchRequest
		}
		if httpDispatchDigest(artifact.Content) != artifact.SHA256 {
			return nil, fmt.Errorf("%w: inline artifact digest mismatch", errHTTPDispatchRequest)
		}
	}
	apply, err := json.Marshal(httpDispatchPortableApply{Target: spec.Target.Slice, Prompt: spec.ApprovedPrompt})
	if err != nil {
		return nil, errHTTPDispatchRequest
	}
	if len(apply)-len(spec.ApprovedPrompt) > remaining {
		return nil, errHTTPDispatchLimit
	}

	var wire httpDispatchPortableRequest
	wire.Schema, wire.SchemaVersion = DispatchRequestSchema, 2
	wire.DispatchID, wire.CreatedAt, wire.Target = spec.DispatchID, spec.CreatedAt.UTC(), spec.Target
	wire.Repository.GitURL = spec.Source.URL
	wire.Repository.LogicalID = "git:" + spec.Source.URL
	wire.Repository.SourceCommit = spec.Source.Commit
	wire.Input.ApplyPayload = string(apply)
	wire.Input.ApplySHA256 = httpDispatchDigest(apply)
	wire.Input.PromptSHA256 = httpDispatchDigest([]byte(spec.ApprovedPrompt))
	wire.Input.ExecutionPolicy = ExecutionPolicy{TaskEvidence: "read_only", Promotion: "caller_only"}
	wire.Input.ContextArtifacts = make([]httpDispatchPortableArtifact, 0, len(spec.Artifacts))
	for _, artifact := range spec.Artifacts {
		wire.Input.ContextArtifacts = append(wire.Input.ContextArtifacts, httpDispatchPortableArtifact{
			ID: artifact.ID, MediaType: artifact.MediaType,
			httpDispatchArtifact: httpDispatchArtifact{
				Kind: "inline", Size: int64(len(artifact.Content)), SHA256: artifact.SHA256,
				Content: base64.StdEncoding.EncodeToString(artifact.Content),
			},
		})
	}
	wire.Requirements.TimeoutSeconds = spec.TimeoutSeconds
	wire.Requirements.MaxAttempts = spec.MaxAttempts
	wire.Requirements.Isolation = "required"
	wire.Requirements.Cancellation = "optional"
	if spec.CancellationRequired {
		wire.Requirements.Cancellation = "required"
	}
	wire.Requirements.Report.Required = spec.ReportRequired
	if spec.Verification != nil {
		wire.Requirements.Verification.Required = true
		wire.Requirements.Verification.httpDispatchVerificationProfile = *spec.Verification
	}
	wire.Options = spec.Options

	frozen, err := json.Marshal(wire)
	if err != nil {
		return nil, errHTTPDispatchRequest
	}
	if len(frozen) > httpDispatchRequestBody {
		return nil, errHTTPDispatchLimit
	}
	var request httpDispatchRequest
	if !decodeHTTPDispatch(frozen, "Request", &request) {
		return nil, errHTTPDispatchRequest
	}
	if _, _, ok := request.validateInput(); !ok {
		return nil, errHTTPDispatchRequest
	}
	if err := checkHTTPDispatchCheckout(ctx, runner, checkout, spec.Source.Commit); err != nil {
		return nil, err
	}
	return frozen, nil
}

func (s httpDispatchBuildSpec) validMetadata() bool {
	if len(s.DispatchID) != 35 || !httpDispatchID.MatchString(s.DispatchID) || s.CreatedAt.IsZero() ||
		len(s.Target.Owner) > 256 || !validHTTPDispatchName(s.Target.Slice, 128) ||
		(s.Target.Thread != nil && !validHTTPDispatchName(*s.Target.Thread, 128)) ||
		s.TimeoutSeconds < 1 || s.TimeoutSeconds > 3600 || s.MaxAttempts < 1 || s.MaxAttempts > 5 {
		return false
	}
	owner, name, _ := strings.Cut(s.Target.Owner, ":")
	return (owner == "task" || owner == "change") && validHTTPDispatchName(name, 256) &&
		(s.Verification == nil || (validHTTPDispatchName(s.Verification.Profile, 128) &&
			(s.Verification.Strength == "smoke" || s.Verification.Strength == "suite"))) &&
		validHTTPDispatchText(s.Options.AgentProvider, 128) && strings.TrimSpace(s.Options.AgentProvider) != "" &&
		validHTTPDispatchText(s.Options.Model, 256) && validHTTPDispatchText(s.Options.Effort, 128)
}

func validHTTPDispatchName(value string, maxBytes int) bool {
	return len(value) <= maxBytes && httpDispatchPortableName.MatchString(value)
}

func validHTTPDispatchText(value string, maxRunes int) bool {
	return len(value) <= maxRunes*utf8.UTFMax && utf8.ValidString(value)
}

func validHTTPDispatchSource(source httpDispatchConfirmedSource) bool {
	// Leave space for the logical ID prefix; do not rewrite or redact a URL.
	if len(source.URL) > 512-len("git:") || (len(source.Commit) != 40 && len(source.Commit) != 64) ||
		!httpDispatchCommit.MatchString(source.Commit) || !strings.HasPrefix(source.URL, "https://") ||
		strings.ContainsAny(source.URL, "@?#\\%") {
		return false
	}
	u, err := url.Parse(source.URL)
	return err == nil && u.Hostname() != "" && strings.HasPrefix(u.Path, "/") && u.Path != "/" &&
		u.EscapedPath() == u.Path && path.Clean(u.Path) == u.Path
}

// This is a point-in-time check of Git-visible changes in a trusted checkout.
// No local remote config or remote-tracking ref participates in source approval.
func checkHTTPDispatchCheckout(ctx context.Context, runner CommandRunner, checkout, commit string) error {
	if runner == nil || checkout == "" {
		return fmt.Errorf("%w: missing local checkout", errHTTPDispatchRequest)
	}
	status, err := runGitBytes(ctx, runner, checkout, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil || len(status) != 0 {
		return fmt.Errorf("%w: local checkout is dirty or could not be inspected", errHTTPDispatchRequest)
	}
	head, err := runGitText(ctx, runner, checkout, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || head != commit {
		return fmt.Errorf("%w: local HEAD does not match the confirmed commit", errHTTPDispatchRequest)
	}
	return nil
}
