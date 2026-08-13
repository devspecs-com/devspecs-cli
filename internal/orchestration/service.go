package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
)

type Service struct {
	Store    StateStore
	Runner   CommandRunner
	Registry *Registry
	Now      func() time.Time
	NewID    func() string
}

type applyOutput struct {
	Target           string            `json:"target"`
	Prompt           string            `json:"prompt"`
	ContextArtifacts []ContextArtifact `json:"context_artifacts"`
}

func NewService(home string) *Service {
	factory := idgen.NewFactory()
	runner := ExecRunner{}
	registry := NewRegistry()
	registry.Register("waspflow", func(options map[string]any) (Driver, error) {
		return NewWaspflowDriver(runner, options)
	})
	return &Service{
		Store:    StateStore{Root: home},
		Runner:   runner,
		Registry: registry,
		Now:      time.Now,
		NewID:    func() string { return factory.NewWithPrefix("handoff_") },
	}
}

func (s *Service) Preflight(ctx context.Context, executionRepo string) (Capabilities, error) {
	driver, err := s.driverForRepo(executionRepo)
	if err != nil {
		return Capabilities{}, err
	}
	return driver.Preflight(ctx)
}

func (s *Service) Dispatch(ctx context.Context, opts DispatchOptions) (HandoffHandle, string, error) {
	if err := validateDispatchOptions(opts); err != nil {
		return HandoffHandle{}, "", err
	}
	executionRepo, err := filepath.Abs(opts.ExecutionRepo)
	if err != nil {
		return HandoffHandle{}, "", err
	}
	driver, err := s.driverForRepo(executionRepo)
	if err != nil {
		return HandoffHandle{}, "", err
	}
	capabilities, err := driver.Preflight(ctx)
	if err != nil {
		return HandoffHandle{}, "", err
	}
	if !capabilities.Isolation {
		return HandoffHandle{}, "", fmt.Errorf("provider %q does not support required isolation", driver.Name())
	}
	repository, err := inspectRepository(ctx, s.Runner, executionRepo)
	if err != nil {
		return HandoffHandle{}, "", err
	}
	applyData, apply, err := s.captureApply(ctx, opts)
	if err != nil {
		return HandoffHandle{}, "", err
	}
	executionPrompt := orchestrationPrompt(apply.Prompt)
	thread := optionalString(opts.Thread)
	request := HandoffRequest{
		Schema:        HandoffSchema,
		SchemaVersion: SchemaVersion,
		HandoffID:     s.NewID(),
		CreatedAt:     s.Now().UTC(),
		Target:        Target{Owner: opts.Target, Slice: apply.Target, Thread: thread},
		Repository:    repository,
		Input: Input{
			ApplyMediaType:        "application/json",
			ApplySHA256:           sha256Bytes(applyData),
			ApplyPayload:          append(json.RawMessage(nil), applyData...),
			ContextArtifacts:      nonNilContextArtifacts(apply.ContextArtifacts),
			ExecutionPromptSHA256: sha256Bytes([]byte(executionPrompt)),
			ExecutionPolicy:       ExecutionPolicy{TaskEvidence: "read_only", Promotion: "caller_only"},
		},
		Requirements: Requirements{
			TimeoutSeconds: opts.TimeoutSeconds,
			MaxAttempts:    1,
			Isolation:      "required",
			Cancellation:   "optional",
			Report:         ReportRequirement{Required: opts.Report != "", Path: opts.Report},
			Verification:   VerifyRequirement{Required: opts.Verify != "", Command: opts.Verify, Strength: opts.VerifyStrength},
		},
	}
	requestPath, requestSHA, err := s.Store.WriteRequest(request)
	if err != nil {
		return HandoffHandle{}, "", err
	}
	providerHandle, err := driver.Dispatch(ctx, request, opts)
	if err != nil {
		return HandoffHandle{}, "", fmt.Errorf("dispatch %s handoff %s failed; request preserved at %s: %w", driver.Name(), request.HandoffID, requestPath, err)
	}
	handle := HandoffHandle{
		Schema:        HandleSchema,
		SchemaVersion: SchemaVersion,
		HandoffID:     request.HandoffID,
		RequestSHA256: requestSHA,
		Request:       ArtifactRef{Kind: "path", Value: requestPath, SHA256: requestSHA},
		Provider:      providerHandle,
	}
	handlePath, err := s.Store.WriteHandle(handle)
	if err != nil {
		return HandoffHandle{}, "", fmt.Errorf("provider dispatched but handle publication failed; recover provider run %s manually: %w", providerHandle.RunID, err)
	}
	return handle, handlePath, nil
}

func (s *Service) Wait(ctx context.Context, identifier string) (HandoffHandle, error) {
	handle, request, driver, err := s.load(ctx, identifier)
	if err != nil {
		return HandoffHandle{}, err
	}
	if err := driver.Wait(ctx, handle.Provider, request.Requirements.TimeoutSeconds); err != nil {
		return HandoffHandle{}, fmt.Errorf("wait did not complete; handoff %s remains recoverable with ds-orchestrate wait %s: %w", handle.HandoffID, handle.HandoffID, err)
	}
	return handle, nil
}

func (s *Service) Finalize(ctx context.Context, identifier string) (HandoffReceipt, string, error) {
	handle, request, driver, err := s.load(ctx, identifier)
	if err != nil {
		return HandoffReceipt{}, "", err
	}
	result, err := driver.Finalize(ctx, handle.Provider, request, s.Store.HandoffDir(handle.HandoffID))
	if err != nil {
		return HandoffReceipt{}, "", fmt.Errorf("finalize handoff %s failed; provider state was preserved when safe: %w", handle.HandoffID, err)
	}
	receipt := HandoffReceipt{
		Schema:        ReceiptSchema,
		SchemaVersion: SchemaVersion,
		ReceiptID:     s.NewID(),
		HandoffID:     handle.HandoffID,
		RequestSHA256: handle.RequestSHA256,
		Provider: ReceiptProvider{
			Name: driver.Name(), AdapterVersion: handle.Provider.AdapterVersion,
			ProviderVersion: result.ProviderVersion, RunID: result.RunID,
		},
		Execution: ReceiptExecution{
			State: result.ExecutionState, Attempts: result.Attempts,
			StartedAt: result.StartedAt, EndedAt: result.EndedAt, Failure: result.Failure,
		},
		Repository: ReceiptRepository{
			LogicalID: request.Repository.LogicalID, EffectiveLocation: result.EffectiveLocation,
			SourceCommit: request.Repository.SourceCommit, ResultCommit: result.ResultCommit,
			ResultStateDigest: result.ResultStateDigest,
		},
		Report: ReceiptReport{State: result.ReportState, Artifact: result.ReportArtifact},
		Verification: ReceiptVerification{
			State: result.VerificationState, Strength: result.VerificationStrength,
			FailureClass: result.VerificationFailureClass, Artifact: result.VerificationArtifact,
		},
		Cancellation: ReceiptCancellation{Supported: result.CancellationSupported, State: "not_requested"},
		Artifacts:    []ArtifactRef{},
	}
	path, err := s.Store.WriteReceipt(receipt)
	if err != nil {
		return HandoffReceipt{}, "", err
	}
	return receipt, path, nil
}

func (s *Service) Receipt(identifier string) (HandoffReceipt, error) {
	path := strings.TrimSpace(identifier)
	if filepath.Ext(path) == "" && !strings.ContainsAny(path, `/\\`) {
		path = s.Store.ReceiptPath(path)
	}
	var receipt HandoffReceipt
	if err := readJSONFile(path, &receipt); err != nil {
		return HandoffReceipt{}, err
	}
	if receipt.Schema != ReceiptSchema || receipt.SchemaVersion != SchemaVersion {
		return HandoffReceipt{}, fmt.Errorf("unsupported orchestration receipt")
	}
	return receipt, nil
}

func (s *Service) load(ctx context.Context, identifier string) (HandoffHandle, HandoffRequest, Driver, error) {
	handle, request, err := s.Store.LoadHandle(identifier)
	if err != nil {
		return HandoffHandle{}, HandoffRequest{}, nil, err
	}
	driver, err := s.driverForRepo(request.Repository.RequestedRoot)
	if err != nil {
		return HandoffHandle{}, HandoffRequest{}, nil, err
	}
	if driver.Name() != handle.Provider.Name {
		return HandoffHandle{}, HandoffRequest{}, nil, fmt.Errorf("configured provider %q does not match handle provider %q", driver.Name(), handle.Provider.Name)
	}
	return handle, request, driver, nil
}

func (s *Service) driverForRepo(executionRepo string) (Driver, error) {
	repoRoot, err := filepath.Abs(executionRepo)
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load orchestration config: %w", err)
	}
	if cfg == nil || strings.TrimSpace(cfg.Integrations.Orchestration.Provider) == "" {
		return nil, fmt.Errorf("orchestration is disabled; configure integrations.orchestration.provider explicitly")
	}
	return s.Registry.Driver(cfg.Integrations.Orchestration.Provider, cfg.Integrations.Orchestration.Options)
}

func (s *Service) captureApply(ctx context.Context, opts DispatchOptions) ([]byte, applyOutput, error) {
	dsExecutable, err := resolveDSExecutable(opts.DSExecutable)
	if err != nil {
		return nil, applyOutput{}, err
	}
	identifier := opts.Target
	if strings.HasPrefix(identifier, "task:") && opts.Thread == "" {
		identifier = strings.TrimPrefix(identifier, "task:")
	}
	args := []string{"apply", identifier, "--repo", opts.TaskRepo, "--json"}
	if opts.Thread != "" {
		args = append(args, "--thread", opts.Thread)
	}
	result, err := s.Runner.Run(ctx, opts.TaskRepo, dsExecutable, args...)
	if err != nil {
		return nil, applyOutput{}, err
	}
	if result.ExitCode != 0 {
		return nil, applyOutput{}, fmt.Errorf("ds apply failed: %s", boundedMessage(result.Stderr))
	}
	if len(result.Stdout) > maxHandoffFileBytes {
		return nil, applyOutput{}, fmt.Errorf("ds apply payload exceeds 2 MiB")
	}
	var output applyOutput
	if err := json.Unmarshal(result.Stdout, &output); err != nil {
		return nil, applyOutput{}, fmt.Errorf("parse ds apply output: %w", err)
	}
	if output.Target == "" || output.Prompt == "" {
		return nil, applyOutput{}, fmt.Errorf("ds apply returned an incompatible payload")
	}
	return result.Stdout, output, nil
}

func validateDispatchOptions(opts DispatchOptions) error {
	if !strings.HasPrefix(opts.Target, "task:") && !strings.HasPrefix(opts.Target, "change:") {
		return fmt.Errorf("--target must use task:<id> or change:<id>")
	}
	if strings.TrimSpace(opts.TaskRepo) == "" || strings.TrimSpace(opts.ExecutionRepo) == "" {
		return fmt.Errorf("--task-repo and --cwd are required")
	}
	if strings.TrimSpace(opts.Lane) == "" {
		return fmt.Errorf("--lane is required")
	}
	if (opts.AgentProvider == "") == (opts.Operation == "") {
		return fmt.Errorf("choose exactly one of --agent-provider or --op")
	}
	if opts.TimeoutSeconds <= 0 || opts.VerifyTimeoutSeconds <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}
	if opts.Prepare != "" && opts.Verify == "" {
		return fmt.Errorf("--prepare requires --verify")
	}
	if opts.VerifyStrength != "" && opts.Verify == "" {
		return fmt.Errorf("--verify-strength requires --verify")
	}
	return nil
}

func resolveDSExecutable(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	current, err := os.Executable()
	if err == nil {
		name := "ds"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		sibling := filepath.Join(filepath.Dir(current), name)
		if _, statErr := os.Stat(sibling); statErr == nil {
			return sibling, nil
		}
	}
	return exec.LookPath("ds")
}

func orchestrationPrompt(prompt string) string {
	return strings.TrimRight(prompt, "\n") + `

---
DevSpecs orchestration lifecycle policy (overrides lifecycle-write instructions above):
- Treat the DevSpecs task/change corpus, including task.json, plans, results, and checkpoints, as read-only.
- Do not run ds task checkpoint or otherwise promote, supersede, or mutate the DevSpecs target.
- Make implementation changes only in the provider's isolated execution workspace.
- Return the decision recommendation and evidence in the required provider report when one is declared; the caller owns any later DevSpecs checkpoint.
`
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func nonNilContextArtifacts(value []ContextArtifact) []ContextArtifact {
	if value == nil {
		return []ContextArtifact{}
	}
	return value
}
