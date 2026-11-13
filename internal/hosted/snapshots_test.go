package hosted

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/hosted/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapshotPolicyFixture() snapshot.Policy {
	return snapshot.Policy{Schema: "devspecs.hosted.snapshot.policy", SchemaVersion: 1, PolicyID: "55555555-5555-4555-8555-555555555555", Roots: []snapshot.Root{{Label: "tasks", Kind: "task_parent", Path: "devspecs/tasks"}}}
}

func snapshotManifestFixture() snapshot.Snapshot {
	content := []byte(strings.Repeat("synthetic committed bytes\n", 1000))
	return snapshot.Snapshot{Schema: snapshot.Schema, SchemaVersion: 1, PolicyID: snapshotPolicyFixture().PolicyID, Commit: strings.Repeat("1", 40), Ref: "refs/heads/main", Records: []snapshot.Record{{Kind: "task", OwnerKind: "repository", OwnerID: repoID, RecordID: "fixture", SchemaVersion: 1, Files: []snapshot.File{{Path: "devspecs/tasks/fixture/task.json", SHA256: snapshot.Hash(content), Content: content}}}}}
}

func snapshotPublishedURL(origin string) string {
	return origin + "/v1/tenants/" + tenantID + "/repos/" + repoID + "/snapshots/66666666-6666-4666-8666-666666666666"
}

func TestHostedSnapshotPolicy_WithPublishOnlyToken_UsesBodylessAuthorizedPolicyRoute(t *testing.T) {
	requests := make(chan *http.Request, 1)
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) { requests <- r; wire(w, 200, snapshotPolicyFixture()) })

	policy, err := client.SnapshotPolicy(t.Context(), approvedRemote(client.origin), serviceToken)

	require.NoError(t, err)
	assert.Equal(t, snapshotPolicyFixture().PolicyID, policy.PolicyID)
	require.Len(t, requests, 1)
	request := <-requests
	assert.Equal(t, http.MethodGet, request.Method)
	assert.Equal(t, "/base/v1/tenants/"+tenantID+"/repos/"+repoID+"/snapshots/policy", request.URL.Path)
	assert.Zero(t, request.ContentLength)
	assert.Equal(t, "Bearer "+serviceToken, request.Header.Get("Authorization"))
}

func TestHostedSnapshotPublish_WithLargeCanonicalBody_SendsExactVersionOneBytes(t *testing.T) {
	bodies := make(chan string, 1)
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		bodies <- string(raw)
		wire(w, 200, PublishedSnapshot{1, "https://" + r.Host + strings.TrimSuffix(r.URL.Path, "/snapshots") + "/snapshots/66666666-6666-4666-8666-666666666666"})
	})
	manifest := snapshotManifestFixture()
	expected, err := snapshot.Canonical(manifest)
	require.NoError(t, err)
	require.Greater(t, len(expected), 16384)

	result, err := client.PublishSnapshot(t.Context(), approvedRemote(client.origin), serviceToken, manifest)

	require.NoError(t, err)
	assert.Equal(t, snapshotPublishedURL(client.origin), result.URL)
	require.Len(t, bodies, 1)
	assert.Equal(t, string(expected), <-bodies)
}

func TestHostedSnapshotPublish_WithSixSecondVerification_UsesSeparateLongBudget(t *testing.T) {
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		timer := time.NewTimer(6 * time.Second)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		wire(w, 200, PublishedSnapshot{1, "https://" + r.Host + r.URL.Path + "/66666666-6666-4666-8666-666666666666"})
	})

	result, err := client.PublishSnapshot(t.Context(), approvedRemote(client.origin), serviceToken, snapshotManifestFixture())

	require.NoError(t, err)
	assert.Equal(t, snapshotPublishedURL(client.origin), result.URL)
	assert.Equal(t, 10*time.Second, client.http.Timeout)
	assert.Equal(t, 50*time.Second, SnapshotPublishTimeout)
}

func TestHostedSnapshotPublish_WithForeignReturnedURL_RejectsWithoutReadingIt(t *testing.T) {
	var calls atomic.Int32
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		wire(w, 200, PublishedSnapshot{1, "https://outside.invalid/snapshots/66666666-6666-4666-8666-666666666666"})
	})

	_, err := client.PublishSnapshot(t.Context(), approvedRemote(client.origin), serviceToken, snapshotManifestFixture())

	require.ErrorIs(t, err, ErrProtocol)
	assert.EqualValues(t, 1, calls.Load())
	assert.NotContains(t, err.Error(), "outside.invalid")
}

func TestHostedSnapshotPublish_WithRedirect_DoesNotForwardBearer(t *testing.T) {
	var forwarded atomic.Int32
	target, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1); wire(w, 200, nil) })
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.origin)
		wire(w, 307, map[string]any{"schema_version": 1, "error": map[string]any{"code": "unavailable"}})
	})

	_, err := client.PublishSnapshot(t.Context(), approvedRemote(client.origin), serviceToken, snapshotManifestFixture())

	require.ErrorIs(t, err, ErrProtocol)
	assert.Zero(t, forwarded.Load())
	assert.NotContains(t, err.Error(), serviceToken)
}

func TestHostedSnapshotPublish_WithLostPostResponse_DoesNotAutomaticallyRetry(t *testing.T) {
	var posts atomic.Int32
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		connection, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			_ = connection.Close()
		}
	})

	_, err := client.PublishSnapshot(t.Context(), approvedRemote(client.origin), serviceToken, snapshotManifestFixture())

	require.ErrorIs(t, err, ErrNetwork)
	assert.EqualValues(t, 1, posts.Load())
}

func TestHostedSnapshotPolicy_WithMetadataOnlyDenial_ReturnsNoCapabilityUpgrade(t *testing.T) {
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 403, map[string]any{"schema_version": 1, "error": map[string]any{"code": "access_denied"}})
	})

	_, err := client.SnapshotPolicy(t.Context(), approvedRemote(client.origin), sessionToken)

	require.ErrorIs(t, err, ErrDenied)
	var refusal *SnapshotError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, "access_denied", refusal.Code)
}

func TestHostedSnapshotPublish_WithConflict_PreservesSafeVersionOneError(t *testing.T) {
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 409, map[string]any{"schema_version": 1, "error": map[string]any{"code": "record_conflict"}})
	})

	_, err := client.PublishSnapshot(t.Context(), approvedRemote(client.origin), serviceToken, snapshotManifestFixture())

	var refusal *SnapshotError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, 409, refusal.Status)
	assert.Equal(t, "record_conflict", refusal.Code)
}

func TestHostedSnapshotPublish_WithProviderSecretInExtraMessage_DoesNotEchoIt(t *testing.T) {
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 403, map[string]any{"schema_version": 1, "error": map[string]any{"code": "access_denied", "message": "private " + serviceToken}})
	})

	_, err := client.PublishSnapshot(t.Context(), approvedRemote(client.origin), serviceToken, snapshotManifestFixture())

	require.Error(t, err)
	assert.NotContains(t, err.Error(), serviceToken)
	assert.NotContains(t, err.Error(), "private")
}

func TestHostedSnapshotPublish_WithInvalidFileDigest_FailsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); wire(w, 200, nil) })
	manifest := snapshotManifestFixture()
	manifest.Records[0].Files[0].SHA256 = strings.Repeat("0", 64)

	_, err := client.PublishSnapshot(t.Context(), approvedRemote(client.origin), serviceToken, manifest)

	require.ErrorIs(t, err, snapshot.ErrInvalid)
	assert.Zero(t, calls.Load())
}

func TestHostedSnapshotPolicy_WithCancelledContext_DoesNotContactService(t *testing.T) {
	var calls atomic.Int32
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); wire(w, 200, nil) })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := client.SnapshotPolicy(ctx, approvedRemote(client.origin), serviceToken)

	require.ErrorIs(t, err, ErrCancelled)
	assert.Zero(t, calls.Load())
}

func TestHostedSnapshotPolicy_WithFilesystemSourceInResponse_RejectsUntrustedAuthority(t *testing.T) {
	policy := snapshotPolicyFixture()
	raw, err := json.Marshal(policy)
	require.NoError(t, err)
	raw = append([]byte(`{"repository_path":"C:/untrusted",`), raw[1:]...)
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	})

	_, err = client.SnapshotPolicy(t.Context(), approvedRemote(client.origin), serviceToken)

	require.ErrorIs(t, err, ErrProtocol)
}
