package snapshot

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonical_WithServiceGolden_ProducesIdenticalVersionOneBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/snapshot-v1.json")
	require.NoError(t, err)
	var manifest Snapshot
	require.NoError(t, json.Unmarshal(raw, &manifest))

	actual, err := Canonical(manifest)

	require.NoError(t, err)
	assert.Equal(t, strings.TrimSuffix(string(raw), "\n"), string(actual))
	assert.Equal(t, "85d4f2cf83491723921e72c2576a967b28aaca534e885a1888ef04d9e92a51bf", Hash(actual))
}

func TestDecodePolicy_WithServiceGolden_PreservesExplicitLabeledRoot(t *testing.T) {
	raw, err := os.ReadFile("testdata/policy-v1.json")
	require.NoError(t, err)

	policy, err := DecodePolicy(raw)

	require.NoError(t, err)
	assert.Equal(t, fixturePolicyID, policy.PolicyID)
	require.Len(t, policy.Roots, 1)
	assert.Equal(t, "planning", policy.Roots[0].Label)
	assert.Equal(t, "task_parent", policy.Roots[0].Kind)
	assert.Equal(t, "custom/planning", policy.Roots[0].Path)
}

func TestDecodePolicy_WithSourceIdentityField_RefusesNonPolicyAuthority(t *testing.T) {
	raw, err := os.ReadFile("testdata/policy-v1.json")
	require.NoError(t, err)
	raw = append([]byte(`{"repository_path":"C:/outside",`), raw[1:]...)

	_, err = DecodePolicy(raw)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestDecodePolicy_WithDuplicateNestedPath_RefusesAmbiguousScope(t *testing.T) {
	raw, err := os.ReadFile("testdata/policy-v1.json")
	require.NoError(t, err)
	data := strings.Replace(string(raw), `"path":"custom/planning"`, `"path":"custom/planning","path":"outside"`, 1)

	_, err = DecodePolicy([]byte(data))

	require.ErrorIs(t, err, ErrInvalid)
}

func TestCanonical_WithMoreThan512Records_RefusesWholeSnapshot(t *testing.T) {
	manifest := Snapshot{Schema: Schema, SchemaVersion: 1, PolicyID: fixturePolicyID, Commit: strings.Repeat("1", 40), Ref: "refs/heads/main", Records: make([]Record, MaxRecords+1)}

	_, err := Canonical(manifest)

	require.ErrorIs(t, err, ErrTooLarge)
}

func TestCheckpointDigest_WithRelocatedFiles_PreservesRoleAndByteIdentity(t *testing.T) {
	original := Record{Kind: "checkpoint", Files: []File{{Path: "old/one.json", Content: []byte(`{"checkpoint_id":"same"}`)}, {Path: "old/one.md", Content: []byte("# Evidence\n")}}}
	expected, err := CheckpointDigest(original)
	require.NoError(t, err)
	moved := Record{Kind: "checkpoint", Files: []File{{Path: "new/corpus/two.md", Content: []byte("# Evidence\n")}, {Path: "new/corpus/two.json", Content: []byte(`{"checkpoint_id":"same"}`)}}}

	actual, err := CheckpointDigest(moved)

	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCheckpointDigest_WithCRLFChange_ChangesImmutableIdentity(t *testing.T) {
	original := Record{Kind: "checkpoint", Files: []File{{Path: "one.json", Content: []byte("{}\n")}, {Path: "one.md", Content: []byte("# Evidence\n")}}}
	expected, err := CheckpointDigest(original)
	require.NoError(t, err)
	changed := Record{Kind: "checkpoint", Files: []File{{Path: "one.json", Content: []byte("{}\n")}, {Path: "one.md", Content: []byte("# Evidence\r\n")}}}

	actual, err := CheckpointDigest(changed)

	require.NoError(t, err)
	assert.NotEqual(t, expected, actual)
}

func emptyFileManifest() Snapshot {
	return Snapshot{Schema: Schema, SchemaVersion: 1, PolicyID: fixturePolicyID, Commit: strings.Repeat("1", 40), Ref: "refs/heads/main", Records: []Record{{Kind: "task", OwnerKind: "repository", OwnerID: "repo-one", RecordID: "empty", SchemaVersion: 1, Files: []File{{Path: "tasks/empty/plan.md", SHA256: Hash([]byte{}), Content: []byte{}}}}}}
}

func TestCanonical_WithEmptyCommittedFile_EncodesEmptyStringAndRawEmptySHA(t *testing.T) {
	manifest := emptyFileManifest()

	raw, err := Canonical(manifest)

	require.NoError(t, err)
	assert.Contains(t, string(raw), `"content_base64":""`)
	assert.Contains(t, string(raw), `"sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`)
}

func TestDecode_WithNullFileContent_RefusesMissingBytes(t *testing.T) {
	raw, err := Canonical(emptyFileManifest())
	require.NoError(t, err)
	data := strings.Replace(string(raw), `"content_base64":""`, `"content_base64":null`, 1)

	_, err = Decode(strings.NewReader(data))

	require.ErrorIs(t, err, ErrInvalid)
}

func TestDecode_WithAbsentFileContent_RefusesMissingBytes(t *testing.T) {
	raw, err := Canonical(emptyFileManifest())
	require.NoError(t, err)
	data := strings.Replace(string(raw), `,"content_base64":""`, "", 1)

	_, err = Decode(strings.NewReader(data))

	require.ErrorIs(t, err, ErrInvalid)
}

func TestDecodePolicy_WithArtifactRootOnTaskParent_RefusesScopeBroadening(t *testing.T) {
	policy := fixturePolicy()
	policy.Roots[0].ArtifactRoot = "planning"
	raw, err := json.Marshal(policy)
	require.NoError(t, err)

	_, err = DecodePolicy(raw)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestDocumentIdentity_WithEmptyLinkHeading_RejectsNonemptyMarkupWithoutText(t *testing.T) {
	data := []byte("# [](/destination)\n")

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrUnsupported)
}

func TestDocumentIdentity_WithEmptyImageHeading_RejectsDestinationWithoutText(t *testing.T) {
	data := []byte("# ![](/image.png)\n")

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrUnsupported)
}

func TestDocumentIdentity_WithTooManyWhitespaceHeadingNodes_PropagatesInvalidBounds(t *testing.T) {
	data := []byte("# " + strings.Repeat("` ` ", 25001) + "\n")

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestDocumentIdentity_AfterFindingHeadingText_StillBoundsRemainingTree(t *testing.T) {
	data := []byte("# Heading\n\n" + strings.Repeat("* item\n", 25001))

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrInvalid)
}
