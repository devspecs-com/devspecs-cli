package orchestration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
)

var errHTTPDriverEvidence = errors.New("HTTP dispatch required report or verification evidence is unavailable or invalid")
var httpDriverSHA1 = regexp.MustCompile(`^[0-9a-f]{40}$`)
var httpDriverSHA256 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type httpDriverBundleArtifact struct {
	ID        string `json:"artifact_id"`
	Kind      string `json:"kind"`
	MediaType string `json:"media_type"`
	Size      int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	URL       string `json:"url"`
}

type httpDriverReceipt struct {
	Base   []byte
	Wire   httpDispatchReceiptAdmissionWire
	Bundle *httpDriverBundleArtifact
	Tree   string
}

// Strip only the agreed normal-driver extension, then use the unchanged strict
// draft-2 schema. The tagged transport continues rejecting these fields.
func decodeHTTPDriverReceipt(raw []byte) (httpDriverReceipt, error) {
	var out httpDriverReceipt
	var object map[string]json.RawMessage
	var repository map[string]json.RawMessage
	if len(raw) > httpDispatchReceiptBody || !strictHTTPDriverJSON(raw, &object) || !strictHTTPDriverJSON(object["repository"], &repository) {
		return out, errHTTPDispatchResponse
	}
	bundle, bundlePresent := repository["result_bundle"]
	tree, treePresent := repository["result_tree"]
	if bundlePresent != treePresent {
		return out, errHTTPDispatchResponse
	}
	out.Base = raw
	if bundlePresent {
		var artifact httpDriverBundleArtifact
		if !strictHTTPDriverJSON(bundle, &artifact) || json.Unmarshal(tree, &out.Tree) != nil || !httpDriverSHA1.MatchString(out.Tree) ||
			artifact.ID != "result-bundle" || artifact.Kind != "https" || artifact.MediaType != "application/x-git-bundle" ||
			artifact.Size < 1 || artifact.Size > httpDriverBundleLimit || !httpDriverSHA256.MatchString(artifact.SHA256) {
			return out, errHTTPDispatchResponse
		}
		out.Bundle = &artifact
		delete(repository, "result_bundle")
		delete(repository, "result_tree")
		var err error
		object["repository"], err = json.Marshal(repository)
		if err != nil {
			return out, errHTTPDispatchResponse
		}
		out.Base, err = json.Marshal(object)
		if err != nil {
			return out, errHTTPDispatchResponse
		}
	}
	if !decodeHTTPDispatch(out.Base, "Receipt", &out.Wire) {
		return out, errHTTPDispatchResponse
	}
	if out.Bundle != nil && (out.Wire.Execution.State != "succeeded" || out.Wire.Repository.ResultCommit == nil || *out.Wire.Repository.ResultCommit == out.Wire.Repository.SourceCommit) {
		return out, errHTTPDispatchResponse
	}
	return out, nil
}

func (d *httpDriver) receipt(ctx context.Context, p httpDriverPending, transport *httpDispatchTransport, accepted *httpDispatchHandle, status *httpDispatchStatus) (httpDriverReceipt, []byte, error) {
	var out httpDriverReceipt
	route, err := transport.dispatchPath(accepted, "/receipt")
	if err != nil {
		return out, nil, err
	}
	_, raw, err := transport.exchange(ctx, http.MethodGet, route, nil, "", p.ID, httpDispatchReceiptBody, http.StatusOK)
	if err != nil {
		return out, nil, err
	}
	out, err = decodeHTTPDriverReceipt(raw)
	if err != nil {
		return out, nil, err
	}
	// No network artifact fetch may precede caller/run/hash/time correlation.
	_, err = admitHTTPDispatchReceiptUsing(ctx, nil, "", p.Frozen, accepted, status, out.Base, nil, func(*httpDispatchReceiptAdmissionWire) error { return nil })
	if err != nil {
		return out, nil, err
	}
	if out.Wire.Execution.State == "succeeded" {
		if p.Trust.ReportRequired && (out.Wire.Report.State != "produced" || out.Wire.Report.Artifact == nil || out.Wire.Report.Artifact.Kind != "inline") {
			return out, nil, errHTTPDriverEvidence
		}
		if p.Trust.VerificationProfile != "" && (out.Wire.Verification.State != "passed" || out.Wire.Verification.Strength != p.Trust.VerificationStrength ||
			out.Wire.Verification.TestOutput == nil || out.Wire.Verification.TestOutput.Kind != "inline") {
			return out, nil, errHTTPDriverEvidence
		}
	}
	return out, raw, nil
}

func (d *httpDriver) saveInlineEvidence(id, suffix string, artifact *httpDispatchArtifact) (*ArtifactRef, error) {
	if artifact == nil || artifact.Kind != "inline" {
		return nil, nil
	}
	if !artifact.validInline() {
		return nil, errHTTPDriverEvidence
	}
	content, err := base64.StdEncoding.Strict().DecodeString(artifact.Content)
	if err != nil {
		return nil, errHTTPDriverEvidence
	}
	dir, err := d.stateDir(true)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	if err := publishHTTPDispatchReceipt(dir, id+suffix, content); err != nil {
		return nil, errHTTPDriverState
	}
	return &ArtifactRef{Kind: "path", Value: filepath.Join(d.store.Root, httpDriverStateDir, id+suffix), SHA256: artifact.SHA256}, nil
}
