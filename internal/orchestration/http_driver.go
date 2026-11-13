package orchestration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"time"
)

var errHTTPDriverGoal = errors.New("HTTP dispatch requires an explicit matching trusted goal and self-contained inline input; native execution options are unsupported")
var errHTTPDriverEdited = errors.New("experimental HTTP dispatch refuses edited results until verified result transfer is available")

type httpDriver struct {
	store        StateStore
	checkout     string
	readOnlyGoal bool
	editedGoal   bool
}

// registerHTTPDriver is the normal Service integration hook. Repository options
// can select a goal, never an endpoint, profile path, token name or credential.
func registerHTTPDriver(registry *Registry, store StateStore) {
	registry.Register("http", func(options map[string]any) (Driver, error) {
		if len(options) != 1 || (options["goal"] != "read_only" && options["goal"] != "edited") {
			return nil, errHTTPDriverGoal
		}
		return &httpDriver{store: store, readOnlyGoal: options["goal"] == "read_only", editedGoal: options["goal"] == "edited"}, nil
	})
}

func bindHTTPDriver(driver Driver, checkout string) {
	if d, ok := driver.(*httpDriver); ok {
		d.checkout = checkout
	}
}

func (d *httpDriver) Name() string { return "http" }

// Service calls Preflight before inspection. This is deliberately local-only;
// authenticated capabilities are checked only after both repositories bind.
func (d *httpDriver) Preflight(ctx context.Context) (Capabilities, error) {
	runner, err := newHTTPDriverLocalGit()
	if err != nil {
		return Capabilities{}, err
	}
	root, err := httpDriverRepositoryRoot(ctx, runner, d.checkout)
	if err != nil {
		if ctx.Err() != nil {
			return Capabilities{}, ctx.Err()
		}
		return Capabilities{}, errHTTPDriverBinding
	}
	d.checkout = root
	if !httpDriverOutside(d.store.Root, root) {
		return Capabilities{}, errHTTPDriverBinding
	}
	_, trust, _, err := loadHTTPDriverProfile(d.checkout)
	if err != nil {
		return Capabilities{}, err
	}
	if trust.ReadOnly != d.readOnlyGoal || trust.AllowEdits != d.editedGoal {
		return Capabilities{}, errHTTPDriverBinding
	}
	if err := checkHTTPDriverBinding(ctx, d.checkout, trust); err != nil {
		return Capabilities{}, err
	}
	return Capabilities{Provider: "http", Isolation: true}, nil
}

func (d *httpDriver) prepare(ctx context.Context, opts DispatchOptions) error {
	if err := d.validateOptions(opts); err != nil {
		return err
	}
	taskRepo, err := httpDriverTaskRoot(opts.TaskRepo)
	if err != nil {
		return err
	}
	runner, err := newHTTPDriverLocalGit()
	if err != nil {
		return err
	}
	root, err := httpDriverRepositoryRoot(ctx, runner, d.checkout)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errHTTPDriverBinding
	}
	d.checkout = root
	if !httpDriverOutside(d.store.Root, root) || !httpDriverOutside(d.store.Root, taskRepo) {
		return errHTTPDriverBinding
	}
	_, trust, _, err := loadHTTPDriverProfile(root, taskRepo)
	if err == nil && (trust.ReadOnly != opts.ReadOnly || trust.AllowEdits != opts.AllowEdits) {
		return errHTTPDriverBinding
	}
	return err
}

func (d *httpDriver) validateOptions(opts DispatchOptions) error {
	if opts.ReadOnly == opts.AllowEdits || opts.ReadOnly != d.readOnlyGoal || opts.AllowEdits != d.editedGoal ||
		opts.TimeoutSeconds < 1 || opts.TimeoutSeconds > 3600 ||
		opts.Operation != "" || opts.Prepare != "" || opts.Verify != "" || opts.Report != "" || opts.VerifyStrength != "" || len(opts.ExplicitNativeFlags) != 0 ||
		opts.MCP != "" || len(opts.ProviderArgs) != 0 || opts.Auto || opts.AcknowledgeDeprecated || opts.AcceptProviderDefault {
		return errHTTPDriverGoal
	}
	return nil
}

func (d *httpDriver) session(ctx context.Context, p httpDriverPending) (*httpDispatchTransport, error) {
	if !time.Now().Before(p.ExpiresAt) {
		return nil, errHTTPDriverExpired
	}
	cfg, trust, roots, err := loadHTTPDriverProfile(p.Checkout, p.TaskRepo)
	if err != nil || trust != p.Trust {
		return nil, errHTTPDriverBinding
	}
	if err := checkHTTPDriverBinding(ctx, p.Checkout, trust); err != nil {
		return nil, err
	}
	endpoint, _ := parseHTTPDispatchEndpoint(trust.Endpoint)
	transport, err := newHTTPDispatchClient(endpoint, os.Getenv(cfg.TokenEnv), roots)
	if err != nil {
		return nil, errHTTPDriverBinding
	}
	if remaining := time.Until(p.ExpiresAt); remaining < transport.callTimeout {
		transport.callTimeout = remaining
	}
	transport.authorize = func(ctx context.Context) error {
		_, current, _, err := loadHTTPDriverProfile(p.Checkout, p.TaskRepo)
		if err != nil || current != p.Trust {
			return errHTTPDriverBinding
		}
		if !time.Now().Before(p.ExpiresAt) {
			return errHTTPDriverExpired
		}
		return checkHTTPDriverBinding(ctx, p.Checkout, current)
	}
	return transport, nil
}

func (d *httpDriver) Dispatch(ctx context.Context, request DispatchRequest, opts DispatchOptions) (ProviderHandle, error) {
	if d.validateOptions(opts) != nil || request.Requirements.MaxAttempts != 1 {
		return ProviderHandle{}, errHTTPDriverGoal
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(request.Requirements.TimeoutSeconds)*time.Second)
	defer cancel()
	checkout, err := httpDriverResolvedPath(request.Repository.RequestedRoot)
	if err != nil {
		return ProviderHandle{}, errHTTPDriverBinding
	}
	taskRepo, err := httpDriverTaskRoot(opts.TaskRepo)
	if err != nil {
		return ProviderHandle{}, errHTTPDriverBinding
	}
	cfg, trust, _, err := loadHTTPDriverProfile(checkout, taskRepo)
	if err != nil || request.Repository.SourceCommit != trust.SourceCommit {
		return ProviderHandle{}, errHTTPDriverBinding
	}
	if trust.ReadOnly != opts.ReadOnly || trust.AllowEdits != opts.AllowEdits {
		return ProviderHandle{}, errHTTPDriverBinding
	}
	if err := checkHTTPDriverBinding(ctx, checkout, trust); err != nil {
		return ProviderHandle{}, err
	}
	if !httpDriverOutside(d.store.Root, checkout) || !httpDriverOutside(d.store.Root, taskRepo) {
		return ProviderHandle{}, errHTTPDriverBinding
	}
	prompt, artifacts, err := httpDriverInput(request)
	if err != nil {
		return ProviderHandle{}, err
	}
	if opts.AllowEdits {
		var apply applyOutput
		if json.Unmarshal(request.Input.ApplyPayload, &apply) != nil {
			return ProviderHandle{}, errHTTPDriverGoal
		}
		prompt = orchestrationPrompt(apply.Prompt) + "\nReturn one result commit whose sole parent is source_commit. Keep all DevSpecs lifecycle paths read-only. Return the edited result through the dispatch result bundle; never promote lifecycle state.\n"
	}
	runner, err := newHTTPDriverLocalGit()
	if err != nil {
		return ProviderHandle{}, err
	}
	frozen, err := buildHTTPDispatchRequest(ctx, runner, checkout, httpDispatchBuildSpec{
		DispatchID: request.DispatchID, CreatedAt: request.CreatedAt,
		Caller: httpDispatchCaller{PrincipalID: cfg.PrincipalID, CredentialID: cfg.CredentialID}, Target: request.Target,
		Source:         &httpDispatchConfirmedSource{URL: trust.SourceURL, Commit: trust.SourceCommit},
		ApprovedPrompt: prompt, Artifacts: artifacts, TimeoutSeconds: request.Requirements.TimeoutSeconds, MaxAttempts: 1,
		ReportRequired: cfg.ReportRequired, Verification: cfg.Verification,
		Options: httpDispatchAgentOptions{AgentProvider: opts.AgentProvider, Model: opts.Model, Effort: opts.Effort},
	})
	if err != nil {
		return ProviderHandle{}, err
	}
	localRequest, err := readOrchestrationFile(d.store.RequestPath(request.DispatchID))
	if err != nil || len(localRequest) > maxDispatchFileBytes {
		return ProviderHandle{}, errHTTPDriverState
	}
	p := httpDriverPending{Version: 1, ID: request.DispatchID, Frozen: frozen, Digest: httpDispatchDigest(frozen),
		V1Digest: sha256Bytes(localRequest), Trust: trust, Checkout: checkout, TaskRepo: taskRepo,
		ExpiresAt: time.Now().UTC().Add(httpDriverRecoveryWindow)}
	transport, err := d.session(ctx, p)
	if err != nil {
		return ProviderHandle{}, err
	}
	defer transport.client.CloseIdleConnections()
	caps, err := transport.capabilities(ctx)
	if err != nil {
		return ProviderHandle{}, err
	}
	if !caps.Ready || caps.IdempotencyRetentionSecs < int(httpDriverRecoveryWindow/time.Second) {
		return ProviderHandle{}, errHTTPDispatchPreflight
	}
	p.Provider = caps.Provider
	if err := d.writeRecord(p.ID, ".pending.json", p); err != nil {
		return ProviderHandle{}, err
	}
	adapter := "http-v2-read-only-1"
	if trust.AllowEdits {
		adapter = "http-v2-bundle-1"
	}
	handle := ProviderHandle{Name: "http", AdapterVersion: adapter, RunID: p.ID, Data: json.RawMessage(`{}`)}
	// Publish the existing v1 recovery authority only after its immutable pending
	// record exists, and before the first possibly-accepted POST.
	if _, err := d.store.WriteHandle(DispatchHandle{Schema: DispatchHandleSchema, SchemaVersion: SchemaVersion,
		DispatchID: p.ID, RequestSHA256: p.V1Digest,
		Request: ArtifactRef{Kind: "path", Value: d.store.RequestPath(p.ID), SHA256: p.V1Digest}, Provider: handle}); err != nil {
		return ProviderHandle{}, errHTTPDriverState
	}
	_, err = d.submit(ctx, p, transport, caps)
	return handle, err
}

func httpDriverInput(request DispatchRequest) (string, []httpDispatchInlineInput, error) {
	if sha256Bytes(request.Input.ApplyPayload) != request.Input.ApplySHA256 {
		return "", nil, errHTTPDriverGoal
	}
	var apply struct {
		Target    string `json:"target"`
		Prompt    string `json:"prompt"`
		Artifacts []struct {
			ID        string  `json:"artifact_id"`
			Revision  string  `json:"revision_id"`
			SHA256    string  `json:"sha256"`
			MediaType string  `json:"media_type"`
			Content   *string `json:"content_base64"`
		} `json:"context_artifacts"`
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(request.Input.ApplyPayload, &apply) != nil || !strictHTTPDriverJSON(request.Input.ApplyPayload, &object) ||
		apply.Target != request.Target.Slice || len(apply.Artifacts) != len(request.Input.ContextArtifacts) {
		return "", nil, errHTTPDriverGoal
	}
	if raw, ok := object["context_artifacts"]; ok && !strictHTTPDriverJSON(raw, &apply.Artifacts) {
		return "", nil, errHTTPDriverGoal
	}
	for key := range object {
		switch key {
		case "target", "prompt", "context_artifacts", "command", "task_id", "target_context", "sibling_targets", "prior_slice_evidence", "thread_context", "input_budget":
		default:
			return "", nil, errHTTPDriverGoal
		}
	}
	var artifacts []httpDispatchInlineInput
	for i, artifact := range apply.Artifacts {
		ref := request.Input.ContextArtifacts[i]
		if artifact.Content == nil || artifact.ID != ref.ArtifactID || artifact.Revision != ref.RevisionID || artifact.SHA256 != ref.SHA256 {
			return "", nil, errHTTPDriverGoal
		}
		content, err := base64.StdEncoding.Strict().DecodeString(*artifact.Content)
		if err != nil || httpDispatchDigest(content) != artifact.SHA256 {
			return "", nil, errHTTPDriverGoal
		}
		artifacts = append(artifacts, httpDispatchInlineInput{ID: artifact.ID, MediaType: artifact.MediaType, SHA256: artifact.SHA256, Content: content})
	}
	return apply.Prompt + "\n\nHTTP experimental read-only goal (overrides any editing instructions): inspect and report only. Do not edit any files, create commits, mutate lifecycle state, or run commands that change the workspace. Return source_commit unchanged as result_commit. If the task requires edits, report unsupported goal, not success.\n", artifacts, nil
}

func (d *httpDriver) submit(ctx context.Context, p httpDriverPending, transport *httpDispatchTransport, caps *httpDispatchCapabilities) (*httpDispatchHandle, error) {
	if caps == nil {
		var err error
		caps, err = transport.capabilities(ctx)
		if err != nil {
			return nil, err
		}
	}
	if caps.Provider != p.Provider || caps.IdempotencyRetentionSecs < int(httpDriverRecoveryWindow/time.Second) {
		return nil, errHTTPDriverBinding
	}
	handle, err := transport.submit(ctx, caps, p.Frozen, nil)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(struct {
		Schema  string `json:"schema"`
		Version int    `json:"schema_version"`
		*httpDispatchHandle
	}{DispatchHandleSchema, 2, handle})
	if err != nil || d.writeRecord(p.ID, ".accepted.json", json.RawMessage(raw)) != nil {
		return nil, errHTTPDriverState
	}
	return handle, nil
}

func (d *httpDriver) accepted(ctx context.Context, p httpDriverPending, transport *httpDispatchTransport, replay bool) (*httpDispatchHandle, error) {
	var raw json.RawMessage
	err := d.readRecord(p.ID, ".accepted.json", httpDispatchSmallBody, &raw)
	if errors.Is(err, os.ErrNotExist) && replay {
		return d.submit(ctx, p, transport, nil)
	}
	if err != nil {
		return nil, err
	}
	var handle httpDispatchHandle
	var request httpDispatchRequest
	if !decodeHTTPDispatch(raw, "Handle", &handle) || !decodeHTTPDispatch(p.Frozen, "Request", &request) ||
		handle.DispatchID != p.ID || handle.RequestSHA256 != p.Digest || handle.Provider.Name != p.Provider {
		return nil, errHTTPDriverState
	}
	handle.transport, handle.repository, handle.caller, handle.maxAttempts = transport, request.Repository, request.Caller, 1
	return &handle, nil
}

func (d *httpDriver) Status(ctx context.Context, handle ProviderHandle) (ProviderExecutionStatus, error) {
	p, err := d.pending(handle)
	if err != nil {
		return ProviderExecutionStatus{}, err
	}
	transport, err := d.session(ctx, p)
	if err != nil {
		return ProviderExecutionStatus{}, err
	}
	defer transport.client.CloseIdleConnections()
	accepted, err := d.accepted(ctx, p, transport, false)
	if errors.Is(err, os.ErrNotExist) {
		return ProviderExecutionStatus{State: "pending"}, nil
	}
	if err != nil {
		return ProviderExecutionStatus{}, err
	}
	status, err := transport.status(ctx, accepted)
	if err != nil {
		return ProviderExecutionStatus{}, err
	}
	return ProviderExecutionStatus{State: status.State}, nil
}

func (d *httpDriver) Wait(ctx context.Context, handle ProviderHandle, timeoutSeconds int) error {
	p, err := d.pending(handle)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	if p.ExpiresAt.Before(deadline) {
		deadline = p.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	transport, err := d.session(ctx, p)
	if err != nil {
		return err
	}
	defer transport.client.CloseIdleConnections()
	accepted, err := d.accepted(ctx, p, transport, true)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		status, err := transport.status(ctx, accepted)
		if err != nil {
			return err
		}
		if terminalHTTPDispatchState(status.State) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (d *httpDriver) Finalize(ctx context.Context, handle ProviderHandle, request DispatchRequest, _ string) (ProviderResult, error) {
	p, err := d.pending(handle)
	if err != nil {
		return ProviderResult{}, err
	}
	deadline := time.Now().Add(time.Duration(request.Requirements.TimeoutSeconds) * time.Second)
	if p.ExpiresAt.Before(deadline) {
		deadline = p.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	local, err := marshalJSON(request)
	if err != nil || sha256Bytes(local) != p.V1Digest {
		return ProviderResult{}, errHTTPDriverState
	}
	transport, err := d.session(ctx, p)
	if err != nil {
		return ProviderResult{}, err
	}
	defer transport.client.CloseIdleConnections()
	accepted, err := d.accepted(ctx, p, transport, true)
	if err != nil {
		return ProviderResult{}, err
	}
	status, err := transport.status(ctx, accepted)
	if err != nil {
		return ProviderResult{}, err
	}
	observation, raw, err := d.receipt(ctx, p, transport, accepted, status)
	if err != nil {
		return ProviderResult{}, err
	}
	wire := observation.Wire
	edited := wire.Repository.ResultCommit != nil && *wire.Repository.ResultCommit != p.Trust.SourceCommit
	var admitted *httpDispatchAdmittedReceipt
	var bundle *httpDriverBundleFact
	if edited {
		if !p.Trust.AllowEdits || p.Trust.ReadOnly {
			return ProviderResult{}, errHTTPDriverEdited
		}
		if err := d.writeRecord(p.ID, ".receipt-binding.json", struct {
			RequestSHA256 string `json:"request_sha256"`
			WireSHA256    string `json:"wire_sha256"`
		}{p.Digest, httpDispatchDigest(raw)}); err != nil {
			return ProviderResult{}, err
		}
		admitted, bundle, err = d.verifyBundle(ctx, p, transport, observation, accepted, status)
	} else {
		if observation.Bundle != nil || len(wire.Evidence.Edited) != 0 || (wire.Execution.State == "succeeded" && wire.Repository.ResultCommit == nil) {
			return ProviderResult{}, errHTTPDriverEdited
		}
		runner, gitErr := newHTTPDriverLocalGit()
		if gitErr != nil {
			return ProviderResult{}, gitErr
		}
		admitted, err = admitHTTPDispatchReceipt(ctx, runner, p.Checkout, p.Frozen, accepted, status, observation.Base, nil)
	}
	if err != nil {
		return ProviderResult{}, err
	}
	admitted.WireSHA256 = httpDispatchDigest(raw)
	var evidence any = admitted
	if bundle != nil {
		evidence = struct {
			*httpDispatchAdmittedReceipt
			Bundle              *httpDriverBundleFact `json:"result_bundle"`
			VerificationProfile string                `json:"verification_profile,omitempty"`
		}{admitted, bundle, p.Trust.VerificationProfile}
	}
	if err := d.writeRecord(p.ID, ".evidence.json", evidence); err != nil {
		return ProviderResult{}, err
	}
	result := ProviderResult{ProviderVersion: "http-v2", RunID: handle.RunID, ExecutionState: admitted.State,
		Attempts: admitted.Attempts, EndedAt: admitted.EndedAt, ResultCommit: admitted.ResultCommit,
		EffectiveLocation: ArtifactRef{Kind: "uri", Value: "urn:devspecs:http-dispatch:" + p.ID},
		ReportState:       admitted.ReportState, VerificationState: admitted.VerificationState,
		VerificationStrength: admitted.VerificationStrength}
	result.ReportArtifact, err = d.saveInlineEvidence(p.ID, ".report", wire.Report.Artifact)
	if err != nil {
		return ProviderResult{}, err
	}
	result.VerificationArtifact, err = d.saveInlineEvidence(p.ID, ".verification", wire.Verification.TestOutput)
	if err != nil {
		return ProviderResult{}, err
	}
	result.CancellationSupported = wire.Cancellation.Supported
	if bundle != nil {
		result.EffectiveLocation = bundle.Artifact
		result.Artifacts = []ArtifactRef{bundle.Artifact}
	}
	if admitted.StartedAt != nil {
		result.StartedAt = *admitted.StartedAt
	}
	if admitted.Failure != nil {
		result.Failure = &Failure{Class: admitted.Failure.Class, Detail: "provider detail omitted"}
	}
	return result, nil
}
