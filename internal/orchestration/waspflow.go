package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const waspflowAdapterVersion = "0.1.0"

type WaspflowDriver struct {
	runner     CommandRunner
	executable string
	home       string
}

type waspflowHandleData struct {
	Lane string `json:"lane"`
}

type waspflowStatus struct {
	LaneUUID       string `json:"lane_uuid"`
	Status         string `json:"status"`
	Result         string `json:"result"`
	CWD            string `json:"cwd"`
	Report         string `json:"report"`
	VerifyState    string `json:"verify_state"`
	VerifyStrength string `json:"verify_strength"`
}

type waspflowNativeReceipt struct {
	LaneUUID        string `json:"lane_uuid"`
	WaspflowVersion string `json:"waspflow_version"`
	Result          string `json:"result"`
	Verify          struct {
		State        string `json:"state"`
		FailureClass string `json:"failure_class"`
		Strength     string `json:"verify_strength"`
	} `json:"verify"`
	Timestamps struct {
		SpawnEpoch    int64 `json:"spawn_epoch"`
		FinalizeEpoch int64 `json:"finalize_epoch"`
	} `json:"timestamps"`
}

func NewWaspflowDriver(runner CommandRunner, options map[string]any) (*WaspflowDriver, error) {
	executable := "waspflow"
	for key, value := range options {
		if key != "executable" {
			return nil, fmt.Errorf("unknown Waspflow orchestration option %q", key)
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("waspflow executable option must be a non-empty string")
		}
		executable = text
	}
	return &WaspflowDriver{runner: runner, executable: executable, home: waspflowHome()}, nil
}

func (d *WaspflowDriver) Name() string {
	return "waspflow"
}

func (d *WaspflowDriver) Preflight(ctx context.Context) (Capabilities, error) {
	result, err := d.runner.Run(ctx, "", d.executable, "doctor")
	if err != nil {
		return Capabilities{}, err
	}
	if result.ExitCode != 0 {
		return Capabilities{}, fmt.Errorf("waspflow preflight failed: %s", commandFailureMessage(result))
	}
	return Capabilities{
		Provider: "waspflow", Isolation: true, Reports: true,
		Verification: true, Cancellation: false,
	}, nil
}

func (d *WaspflowDriver) Dispatch(ctx context.Context, request DispatchRequest, opts DispatchOptions) (ProviderHandle, error) {
	prompt, err := requestExecutionPrompt(request)
	if err != nil {
		return ProviderHandle{}, err
	}
	args := []string{"spawn", "--lane", opts.Lane, "--cwd", request.Repository.RequestedRoot, "--isolate"}
	if opts.MCP != "" {
		args = append(args, "--mcp", opts.MCP)
	}
	if opts.AgentProvider != "" {
		args = append(args, "--provider", opts.AgentProvider)
	}
	if opts.Operation != "" {
		args = append(args, "--op", opts.Operation)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Effort != "" {
		args = append(args, "--effort", opts.Effort)
	}
	if opts.AcceptProviderDefault {
		args = append(args, "--accept-provider-default")
	}
	if opts.Auto {
		args = append(args, "--auto")
	}
	if opts.AcknowledgeDeprecated {
		args = append(args, "--ack-deprecated")
	}
	if opts.Report != "" {
		args = append(args, "--report", opts.Report)
	}
	if opts.Verify != "" {
		args = append(args, "--verify", opts.Verify, "--verify-name", opts.VerifyName, "--verify-timeout", strconv.Itoa(opts.VerifyTimeoutSeconds))
		if opts.VerifyStrength != "" {
			args = append(args, "--verify-strength", opts.VerifyStrength)
		}
		if opts.Prepare != "" {
			args = append(args, "--prepare", opts.Prepare)
		}
	}
	for _, providerArg := range opts.ProviderArgs {
		args = append(args, "--arg", providerArg)
	}
	args = append(args, "--", prompt)
	result, err := d.runner.Run(ctx, request.Repository.RequestedRoot, d.executable, args...)
	if err != nil {
		return ProviderHandle{}, err
	}
	if result.ExitCode != 0 {
		return ProviderHandle{}, fmt.Errorf("waspflow spawn failed: %s", commandFailureMessage(result))
	}
	status, err := d.status(ctx, opts.Lane)
	if err != nil {
		return ProviderHandle{}, fmt.Errorf("waspflow spawned lane %q but its status could not be captured; inspect it manually: %w", opts.Lane, err)
	}
	data, err := json.Marshal(waspflowHandleData{Lane: opts.Lane})
	if err != nil {
		return ProviderHandle{}, err
	}
	runID := status.LaneUUID
	if runID == "" {
		runID = opts.Lane
	}
	return ProviderHandle{
		Name: "waspflow", AdapterVersion: waspflowAdapterVersion,
		RunID: runID, Data: data,
	}, nil
}

func (d *WaspflowDriver) Status(ctx context.Context, handle ProviderHandle) (ProviderExecutionStatus, error) {
	data, err := parseWaspflowHandle(handle)
	if err != nil {
		return ProviderExecutionStatus{}, err
	}
	status, err := d.status(ctx, data.Lane)
	if err != nil {
		return ProviderExecutionStatus{}, err
	}
	return ProviderExecutionStatus{
		State:       classifyWaspflowStatus(status.Status, status.Result),
		NativeState: status.Status,
		Result:      status.Result,
	}, nil
}

func (d *WaspflowDriver) Wait(ctx context.Context, handle ProviderHandle, timeoutSeconds int) error {
	data, err := parseWaspflowHandle(handle)
	if err != nil {
		return err
	}
	result, err := d.runner.Run(ctx, "", d.executable, "wait", data.Lane, "--timeout", strconv.Itoa(timeoutSeconds))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("waspflow wait interrupted; lane %q was not cancelled or reaped: %w", data.Lane, err)
		}
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("waspflow wait exited %d; lane %q was not cancelled or reaped: %s", result.ExitCode, data.Lane, commandFailureMessage(result))
	}
	return nil
}

func (d *WaspflowDriver) Finalize(ctx context.Context, handle ProviderHandle, request DispatchRequest, stateDir string) (ProviderResult, error) {
	data, err := parseWaspflowHandle(handle)
	if err != nil {
		return ProviderResult{}, err
	}
	status, err := d.status(ctx, data.Lane)
	if err != nil {
		return ProviderResult{}, err
	}
	if status.CWD == "" {
		return ProviderResult{}, fmt.Errorf("waspflow lane %q did not expose its effective workspace", data.Lane)
	}

	var reportArtifact *ArtifactRef
	reportState := "not_required"
	if request.Requirements.Report.Required {
		if status.Report == "" {
			return ProviderResult{}, fmt.Errorf("required report path is absent; lane %q was preserved", data.Lane)
		}
		reportArtifact, err = copyBoundedArtifact(status.Report, filepath.Join(stateDir, "provider-report"), 1<<20)
		if err != nil {
			return ProviderResult{}, fmt.Errorf("required report is unavailable; lane %q was preserved: %w", data.Lane, err)
		}
		reportState = "produced"
	}

	if request.Requirements.Verification.Required {
		verifyResult, runErr := d.runner.Run(ctx, "", d.executable, "verify", data.Lane, "--json")
		if runErr != nil {
			return ProviderResult{}, runErr
		}
		if verifyResult.ExitCode != 0 && verifyResult.ExitCode != 2 {
			return ProviderResult{}, fmt.Errorf("waspflow verification could not complete; lane %q was preserved: %s", data.Lane, commandFailureMessage(verifyResult))
		}
	}
	status, err = d.status(ctx, data.Lane)
	if err != nil {
		return ProviderResult{}, err
	}
	resultCommit, resultDigest, err := resultRepositoryState(ctx, d.runner, status.CWD)
	if err != nil {
		return ProviderResult{}, fmt.Errorf("capture provider workspace before cleanup: %w", err)
	}

	var verificationArtifact *ArtifactRef
	verifyPath := filepath.Join(d.home, "lanes", data.Lane, "verify-result.json")
	if _, statErr := os.Stat(verifyPath); statErr == nil {
		verificationArtifact, err = copyBoundedArtifact(verifyPath, filepath.Join(stateDir, "verify-result.json"), 1<<20)
		if err != nil {
			return ProviderResult{}, err
		}
	}

	reapResult, runErr := d.runner.Run(ctx, "", d.executable, "reap", data.Lane)
	if runErr != nil {
		return ProviderResult{}, runErr
	}
	nativePath := filepath.Join(d.home, "lanes", data.Lane, "receipt.json")
	var native waspflowNativeReceipt
	if err := readJSONFile(nativePath, &native); err != nil {
		if reapResult.ExitCode != 0 {
			return ProviderResult{}, fmt.Errorf("waspflow reap exited %d without a readable native receipt; inspect lane %q: %s", reapResult.ExitCode, data.Lane, commandFailureMessage(reapResult))
		}
		return ProviderResult{}, fmt.Errorf("read waspflow native receipt: %w", err)
	}
	executionState, failure := classifyWaspflowResult(native.Result)
	verificationState := native.Verify.State
	if verificationState == "" {
		if request.Requirements.Verification.Required {
			verificationState = status.VerifyState
		} else {
			verificationState = "skipped"
		}
	}
	strength := native.Verify.Strength
	if strength == "" {
		strength = status.VerifyStrength
	}
	return ProviderResult{
		ProviderVersion:   native.WaspflowVersion,
		RunID:             firstNonEmpty(native.LaneUUID, handle.RunID),
		ExecutionState:    executionState,
		Attempts:          1,
		StartedAt:         time.Unix(native.Timestamps.SpawnEpoch, 0).UTC(),
		EndedAt:           time.Unix(native.Timestamps.FinalizeEpoch, 0).UTC(),
		Failure:           failure,
		EffectiveLocation: ArtifactRef{Kind: "path", Value: status.CWD},
		ResultCommit:      resultCommit, ResultStateDigest: resultDigest,
		ReportState: reportState, ReportArtifact: reportArtifact,
		VerificationState: verificationState, VerificationStrength: strength,
		VerificationFailureClass: firstNonEmpty(native.Verify.FailureClass, "none"),
		VerificationArtifact:     verificationArtifact,
		CancellationSupported:    false,
	}, nil
}

func (d *WaspflowDriver) status(ctx context.Context, lane string) (waspflowStatus, error) {
	result, err := d.runner.Run(ctx, "", d.executable, "status", lane)
	if err != nil {
		return waspflowStatus{}, err
	}
	if result.ExitCode != 0 {
		return waspflowStatus{}, fmt.Errorf("waspflow status failed: %s", commandFailureMessage(result))
	}
	var status waspflowStatus
	if err := json.Unmarshal(result.Stdout, &status); err != nil {
		return waspflowStatus{}, fmt.Errorf("parse waspflow status: %w", err)
	}
	return status, nil
}

func parseWaspflowHandle(handle ProviderHandle) (waspflowHandleData, error) {
	if handle.Name != "waspflow" {
		return waspflowHandleData{}, fmt.Errorf("waspflow driver cannot consume provider %q", handle.Name)
	}
	var data waspflowHandleData
	if err := json.Unmarshal(handle.Data, &data); err != nil {
		return waspflowHandleData{}, fmt.Errorf("parse waspflow handle: %w", err)
	}
	if data.Lane == "" {
		return waspflowHandleData{}, fmt.Errorf("waspflow handle is missing lane identity")
	}
	return data, nil
}

func requestExecutionPrompt(request DispatchRequest) (string, error) {
	var apply applyOutput
	if err := json.Unmarshal(request.Input.ApplyPayload, &apply); err != nil {
		return "", fmt.Errorf("parse frozen ds apply payload: %w", err)
	}
	if apply.Prompt == "" {
		return "", fmt.Errorf("frozen ds apply payload has no prompt")
	}
	return orchestrationPrompt(apply.Prompt), nil
}

func classifyWaspflowStatus(status, result string) string {
	if result != "" {
		executionState, _ := classifyWaspflowResult(result)
		return executionState
	}
	switch status {
	case "live":
		return "running"
	case "exited":
		return "ready"
	case "parked":
		return "paused"
	case "escalate_failed":
		return "failed"
	case "reaped":
		return "unknown"
	default:
		return "unknown"
	}
}

func classifyWaspflowResult(value string) (string, *Failure) {
	switch value {
	case "succeeded", "verified", "recovered":
		return "succeeded", nil
	case "":
		return "failed", &Failure{Class: "provider_result", Detail: "missing Waspflow result"}
	default:
		return "failed", &Failure{Class: "provider_result", Detail: value}
	}
}

func copyBoundedArtifact(source, target string, maxBytes int) (*ArtifactRef, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("artifact %s exceeds %d bytes", source, maxBytes)
	}
	if err := writeFileAtomic(target, data, 0o600); err != nil {
		return nil, err
	}
	return &ArtifactRef{Kind: "path", Value: target, SHA256: sha256Bytes(data)}, nil
}

func commandFailureMessage(result CommandResult) string {
	if message := boundedMessage(result.Stderr); message != "" {
		return message
	}
	return boundedMessage(result.Stdout)
}

func waspflowHome() string {
	if value := strings.TrimSpace(os.Getenv("WASPFLOW_HOME")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); value != "" {
		return filepath.Join(value, "waspflow")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "state", "waspflow")
	}
	return filepath.Join(home, ".local", "state", "waspflow")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
