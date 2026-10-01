package orchestration

import (
	"encoding/json"
	"time"
)

const (
	DispatchRequestSchema = "devspecs.dispatch.request"
	DispatchHandleSchema  = "devspecs.dispatch.handle"
	DispatchReceiptSchema = "devspecs.dispatch.receipt"
	DispatchStatusSchema  = "devspecs.dispatch.status"
	SchemaVersion         = 1
)

type Target struct {
	Owner  string  `json:"owner"`
	Slice  string  `json:"slice"`
	Thread *string `json:"thread"`
}

type Repository struct {
	LogicalID         string `json:"logical_id"`
	RequestedRoot     string `json:"requested_root"`
	SourceCommit      string `json:"source_commit"`
	SourceState       string `json:"source_state"`
	SourceStateDigest string `json:"source_state_digest"`
}

type ContextArtifact struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
	SHA256     string `json:"sha256"`
}

type Input struct {
	ApplyMediaType        string            `json:"apply_media_type"`
	ApplySHA256           string            `json:"apply_sha256"`
	ApplyPayload          json.RawMessage   `json:"apply_payload"`
	ContextArtifacts      []ContextArtifact `json:"context_artifacts"`
	ExecutionPromptSHA256 string            `json:"execution_prompt_sha256"`
	ExecutionPolicy       ExecutionPolicy   `json:"execution_policy"`
}

type ExecutionPolicy struct {
	TaskEvidence string `json:"task_evidence"`
	Promotion    string `json:"promotion"`
}

type Requirements struct {
	TimeoutSeconds int               `json:"timeout_seconds"`
	MaxAttempts    int               `json:"max_attempts"`
	Isolation      string            `json:"isolation"`
	Cancellation   string            `json:"cancellation"`
	Report         ReportRequirement `json:"report"`
	Verification   VerifyRequirement `json:"verification"`
}

type ReportRequirement struct {
	Required bool   `json:"required"`
	Path     string `json:"path,omitempty"`
}

type VerifyRequirement struct {
	Required bool   `json:"required"`
	Command  string `json:"command,omitempty"`
	Strength string `json:"strength,omitempty"`
}

type DispatchRequest struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schema_version"`
	DispatchID    string       `json:"dispatch_id"`
	CreatedAt     time.Time    `json:"created_at"`
	Target        Target       `json:"target"`
	Repository    Repository   `json:"repository"`
	Input         Input        `json:"input"`
	Requirements  Requirements `json:"requirements"`
}

type ArtifactRef struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	SHA256 string `json:"sha256,omitempty"`
}

type ProviderHandle struct {
	Name            string          `json:"name"`
	AdapterVersion  string          `json:"adapter_version"`
	ProviderVersion string          `json:"provider_version,omitempty"`
	RunID           string          `json:"run_id"`
	Data            json.RawMessage `json:"data"`
}

type DispatchHandle struct {
	Schema        string         `json:"schema"`
	SchemaVersion int            `json:"schema_version"`
	DispatchID    string         `json:"dispatch_id"`
	RequestSHA256 string         `json:"request_sha256"`
	Request       ArtifactRef    `json:"request"`
	Provider      ProviderHandle `json:"provider"`
}

type Capabilities struct {
	Provider     string `json:"provider"`
	Isolation    bool   `json:"isolation"`
	Reports      bool   `json:"reports"`
	Verification bool   `json:"verification"`
	Cancellation bool   `json:"cancellation"`
}

type ProviderExecutionStatus struct {
	State       string
	NativeState string
	Result      string
}

type DispatchStatusProvider struct {
	Name        string `json:"name"`
	RunID       string `json:"run_id"`
	NativeState string `json:"native_state,omitempty"`
	Result      string `json:"result,omitempty"`
}

type DispatchStatus struct {
	Schema        string                 `json:"schema"`
	SchemaVersion int                    `json:"schema_version"`
	DispatchID    string                 `json:"dispatch_id"`
	State         string                 `json:"state"`
	Provider      DispatchStatusProvider `json:"provider"`
	Receipt       *ArtifactRef           `json:"receipt,omitempty"`
}

type ProviderResult struct {
	ProviderVersion          string
	RunID                    string
	ExecutionState           string
	Attempts                 int
	StartedAt                time.Time
	EndedAt                  time.Time
	Failure                  *Failure
	EffectiveLocation        ArtifactRef
	ResultCommit             *string
	ResultStateDigest        string
	ReportState              string
	ReportArtifact           *ArtifactRef
	VerificationState        string
	VerificationStrength     string
	VerificationFailureClass string
	VerificationArtifact     *ArtifactRef
	CancellationSupported    bool
}

type Failure struct {
	Class  string `json:"class"`
	Detail string `json:"detail"`
}

type ReceiptProvider struct {
	Name            string `json:"name"`
	AdapterVersion  string `json:"adapter_version"`
	ProviderVersion string `json:"provider_version"`
	RunID           string `json:"run_id"`
}

type ReceiptExecution struct {
	State     string    `json:"state"`
	Attempts  int       `json:"attempts"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Failure   *Failure  `json:"failure"`
}

type ReceiptRepository struct {
	LogicalID         string      `json:"logical_id"`
	EffectiveLocation ArtifactRef `json:"effective_location"`
	SourceCommit      string      `json:"source_commit"`
	ResultCommit      *string     `json:"result_commit"`
	ResultStateDigest string      `json:"result_state_digest"`
}

type ReceiptReport struct {
	State    string       `json:"state"`
	Artifact *ArtifactRef `json:"artifact"`
}

type ReceiptVerification struct {
	State        string       `json:"state"`
	Strength     string       `json:"strength"`
	FailureClass string       `json:"failure_class"`
	Artifact     *ArtifactRef `json:"artifact"`
}

type ReceiptCancellation struct {
	Supported bool   `json:"supported"`
	State     string `json:"state"`
}

type DispatchReceipt struct {
	Schema        string              `json:"schema"`
	SchemaVersion int                 `json:"schema_version"`
	ReceiptID     string              `json:"receipt_id"`
	DispatchID    string              `json:"dispatch_id"`
	RequestSHA256 string              `json:"request_sha256"`
	Provider      ReceiptProvider     `json:"provider"`
	Execution     ReceiptExecution    `json:"execution"`
	Repository    ReceiptRepository   `json:"repository"`
	Report        ReceiptReport       `json:"report"`
	Verification  ReceiptVerification `json:"verification"`
	Cancellation  ReceiptCancellation `json:"cancellation"`
	Artifacts     []ArtifactRef       `json:"artifacts"`
}

type DispatchOptions struct {
	TaskRepo              string
	Target                string
	Thread                string
	ExecutionRepo         string
	Lane                  string
	AgentProvider         string
	Operation             string
	Model                 string
	Effort                string
	AcceptProviderDefault bool
	Auto                  bool
	AcknowledgeDeprecated bool
	MCP                   string
	Report                string
	Verify                string
	VerifyName            string
	VerifyTimeoutSeconds  int
	VerifyStrength        string
	Prepare               string
	ProviderArgs          []string
	TimeoutSeconds        int
	DSExecutable          string
}
