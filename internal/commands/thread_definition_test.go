package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadThreadDefinition_WhenYAMLContainsUnknownField_ReturnsError(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), threadDefinitionFilename)
	require.NoError(t, os.WriteFile(path, []byte("schema_version: 1\nrevision: 1\nunknown: true\n"), 0o644))

	// Act
	_, err := readThreadDefinition(path)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "field unknown not found")
}

func TestReadThreadDefinition_WhenYAMLContainsMultipleDocuments_ReturnsError(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), threadDefinitionFilename)
	body := "schema_version: 1\nrevision: 1\nowner:\n  kind: task\n  task_id: threaded-task\nthreads:\n  - key: agent-a\n    targets:\n      - target: F01\n---\nextra: document\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	// Act
	_, err := readThreadDefinition(path)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "multiple YAML documents")
}

func TestWriteThreadDefinition_WhenDefinitionIsValid_PublishesReadableYAML(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "nested", threadDefinitionFilename)
	definition := validRepoThreadDefinition()

	// Act
	err := writeThreadDefinition(path, definition)

	// Assert
	require.NoError(t, err)
	written, err := readThreadDefinition(path)
	require.NoError(t, err)
	assert.Equal(t, 1, written.SchemaVersion)
	assert.Equal(t, 1, written.Revision)
	assert.Equal(t, threadOwnerTask, written.Owner.Kind)
	assert.Equal(t, "threaded-task", written.Owner.TaskID)
	assert.Len(t, written.Threads, 2)
	assert.Equal(t, "agent-a", written.Threads[0].Key)
	assert.Len(t, written.Threads[0].Targets, 1)
	assert.Equal(t, "F01", written.Threads[0].Targets[0].Target)
	assert.Equal(t, "both-a", written.Threads[1].Key)
	assert.Len(t, written.Threads[1].After, 1)
	assert.Equal(t, "agent-a", written.Threads[1].After[0])
}

func TestPublishRepoThreadDefinition_WhenGlobalDatabaseIsAbsent_PreservesDurableDefinition(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	taskWorkspace := filepath.Join(t.TempDir(), "task")
	path := repoThreadDefinitionPath(taskWorkspace)
	definition := validRepoThreadDefinition()
	manifest := validThreadTaskManifest()

	// Act
	err := publishRepoThreadDefinition(t.Context(), taskWorkspace, definition, manifest, nil)
	written, readErr := readThreadDefinition(path)
	_, databaseErr := os.Stat(filepath.Join(home, "devspecs.db"))

	// Assert
	require.NoError(t, err)
	require.NoError(t, readErr)
	assert.True(t, os.IsNotExist(databaseErr))
	assert.Equal(t, "threaded-task", written.Owner.TaskID)
	assert.Len(t, written.Threads, 2)
}

func TestPublishWorkspaceThreadDefinition_WhenChildDatabasesAreAbsent_PreservesCrossRepoTargets(t *testing.T) {
	// Arrange
	t.Setenv("DEVSPECS_HOME", filepath.Join(t.TempDir(), "home"))
	workspaceRoot := t.TempDir()
	definition := validWorkspaceThreadDefinition()
	validation := validWorkspaceThreadValidationContext()
	path := workspaceThreadDefinitionPath(workspaceRoot, validation.Manifest, validation.Change.ID)

	// Act
	err := publishWorkspaceThreadDefinition(t.Context(), workspaceRoot, definition, validation, nil)
	written, readErr := readThreadDefinition(path)

	// Assert
	require.NoError(t, err)
	require.NoError(t, readErr)
	assert.Len(t, written.Threads, 1)
	assert.Len(t, written.Threads[0].Targets, 2)
	assert.Equal(t, "api", written.Threads[0].Targets[0].Repo)
	assert.Equal(t, "api-task", written.Threads[0].Targets[0].Task)
	assert.Equal(t, "web", written.Threads[0].Targets[1].Repo)
	assert.Equal(t, "web-task", written.Threads[0].Targets[1].Task)
}

func TestPublishRepoThreadDefinition_WhenContextValidationFails_LeavesExistingBytesUntouched(t *testing.T) {
	// Arrange
	t.Setenv("DEVSPECS_HOME", filepath.Join(t.TempDir(), "home"))
	taskWorkspace := filepath.Join(t.TempDir(), "task")
	path := repoThreadDefinitionPath(taskWorkspace)
	initial := validRepoThreadDefinition()
	require.NoError(t, writeThreadDefinition(path, initial))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	invalid := validRepoThreadDefinition()
	invalid.Revision = 2
	invalid.Threads[0].Targets[0].Target = "F99"
	manifest := validThreadTaskManifest()

	// Act
	publishErr := publishRepoThreadDefinition(t.Context(), taskWorkspace, invalid, manifest, nil)
	after, readErr := os.ReadFile(path)

	// Assert
	require.Error(t, publishErr)
	assert.ErrorContains(t, publishErr, "F99")
	require.NoError(t, readErr)
	assert.Equal(t, string(before), string(after))
}

func TestPublishRepoThreadDefinition_WhenRevisionDoesNotAdvance_LeavesExistingBytesUntouched(t *testing.T) {
	// Arrange
	t.Setenv("DEVSPECS_HOME", filepath.Join(t.TempDir(), "home"))
	taskWorkspace := filepath.Join(t.TempDir(), "task")
	path := repoThreadDefinitionPath(taskWorkspace)
	initial := validRepoThreadDefinition()
	require.NoError(t, writeThreadDefinition(path, initial))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	stale := validRepoThreadDefinition()
	manifest := validThreadTaskManifest()

	// Act
	publishErr := publishRepoThreadDefinition(t.Context(), taskWorkspace, stale, manifest, nil)
	after, readErr := os.ReadFile(path)

	// Assert
	require.Error(t, publishErr)
	assert.ErrorContains(t, publishErr, "revision must advance")
	require.NoError(t, readErr)
	assert.Equal(t, string(before), string(after))
}

func TestValidateRepoThreadDefinition_WhenTaskIsStandalone_AcceptsLocalTargets(t *testing.T) {
	// Arrange
	definition := validRepoThreadDefinition()
	manifest := validThreadTaskManifest()

	// Act
	err := validateRepoThreadDefinition(definition, manifest)

	// Assert
	require.NoError(t, err)
}

func TestValidateRepoThreadDefinition_WhenTaskIsInWorkspaceButUnlinked_AcceptsRepoOwner(t *testing.T) {
	// Arrange
	definition := validRepoThreadDefinition()
	manifest := validThreadTaskManifest()
	manifest.WorkspaceID = "umbrella"
	manifest.WorkspaceRoot = t.TempDir()

	// Act
	err := validateRepoThreadDefinition(definition, manifest)

	// Assert
	require.NoError(t, err)
}

func TestValidateRepoThreadDefinition_WhenTaskIsLinkedToChange_RejectsCompetingOwner(t *testing.T) {
	// Arrange
	definition := validRepoThreadDefinition()
	manifest := validThreadTaskManifest()
	manifest.ParentChange = "change-1"

	// Act
	err := validateRepoThreadDefinition(definition, manifest)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "change-1")
	assert.ErrorContains(t, err, "owns its thread graph")
}

func TestValidateRepoThreadDefinition_WhenTargetIsAssignedTwice_RejectsAmbiguousMembership(t *testing.T) {
	// Arrange
	definition := validRepoThreadDefinition()
	definition.Threads[1].Targets[0].Target = "F01"
	manifest := validThreadTaskManifest()

	// Act
	err := validateRepoThreadDefinition(definition, manifest)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "assigned to both")
}

func TestValidateRepoThreadDefinition_WhenDependenciesContainCycle_RejectsGraph(t *testing.T) {
	// Arrange
	definition := validRepoThreadDefinition()
	definition.Threads[0].After = []string{"both-a"}
	manifest := validThreadTaskManifest()

	// Act
	err := validateRepoThreadDefinition(definition, manifest)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "contains a cycle")
}

func TestValidateRepoThreadDefinition_WhenTargetIsFollowUp_RejectsDirectAssignment(t *testing.T) {
	// Arrange
	definition := validRepoThreadDefinition()
	definition.Threads[0].Targets[0].Target = "F01-1"
	manifest := validThreadTaskManifest()
	manifest.Artifacts.Slices = append(manifest.Artifacts.Slices, taskSliceArtifact{
		ID:       "F01-1",
		Title:    "Follow up",
		Kind:     "follow_up",
		ParentID: "F01",
	})

	// Act
	err := validateRepoThreadDefinition(definition, manifest)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "inherits")
	assert.ErrorContains(t, err, "cannot be assigned directly")
}

func TestValidateRepoThreadDefinition_WhenTargetIsSeriesCloseout_RejectsAssignment(t *testing.T) {
	// Arrange
	definition := validRepoThreadDefinition()
	definition.Threads[0].Targets[0].Target = "F"
	manifest := validThreadTaskManifest()

	// Act
	err := validateRepoThreadDefinition(definition, manifest)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "series closeout")
}

func TestValidateWorkspaceThreadDefinition_WhenTargetsSpanLinkedRepos_AcceptsChangeOwner(t *testing.T) {
	// Arrange
	definition := validWorkspaceThreadDefinition()
	context := validWorkspaceThreadValidationContext()

	// Act
	err := validateWorkspaceThreadDefinition(definition, context)

	// Assert
	require.NoError(t, err)
}

func TestValidateWorkspaceThreadDefinition_WhenChildBacklinkDisagrees_RejectsTarget(t *testing.T) {
	// Arrange
	definition := validWorkspaceThreadDefinition()
	context := validWorkspaceThreadValidationContext()
	child := context.TaskManifests[workspaceThreadTaskKey("api", "api-task")]
	child.ParentChange = "other-change"
	context.TaskManifests[workspaceThreadTaskKey("api", "api-task")] = child

	// Act
	err := validateWorkspaceThreadDefinition(definition, context)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "does not point back")
}

func TestValidateWorkspaceThreadDefinition_WhenLinkedTaskHasRepoDefinition_RejectsCompetingAuthority(t *testing.T) {
	// Arrange
	definition := validWorkspaceThreadDefinition()
	context := validWorkspaceThreadValidationContext()
	context.RepoDefinitionExists[workspaceThreadTaskKey("api", "api-task")] = true

	// Act
	err := validateWorkspaceThreadDefinition(definition, context)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "competing repo-local thread authority")
}

func TestValidateThreadDefinitionMutation_WhenStartedLaneIsExtended_AcceptsAppendOnlyChange(t *testing.T) {
	// Arrange
	before := validRepoThreadDefinition()
	after := validRepoThreadDefinition()
	after.Revision = 2
	after.Threads[0].Targets = append(after.Threads[0].Targets, threadTargetReference{Target: "F03"})
	history := map[string]bool{"f01": true}

	// Act
	err := validateThreadDefinitionMutation(before, after, history)

	// Assert
	require.NoError(t, err)
}

func TestValidateThreadDefinitionMutation_WhenStartedLaneIsReordered_RejectsChange(t *testing.T) {
	// Arrange
	before := validRepoThreadDefinition()
	before.Threads[0].Targets = append(before.Threads[0].Targets, threadTargetReference{Target: "F03"})
	after := validRepoThreadDefinition()
	after.Revision = 2
	after.Threads[0].Targets = []threadTargetReference{{Target: "F03"}, {Target: "F01"}}
	history := map[string]bool{"f01": true}

	// Act
	err := validateThreadDefinitionMutation(before, after, history)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "may only be extended")
}

func TestValidateThreadDefinitionMutation_WhenStartedLaneDependenciesChange_RejectsChange(t *testing.T) {
	// Arrange
	before := validRepoThreadDefinition()
	after := validRepoThreadDefinition()
	after.Revision = 2
	after.Threads[0].After = []string{"both-a"}
	history := map[string]bool{"f01": true}

	// Act
	err := validateThreadDefinitionMutation(before, after, history)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "dependencies are pinned")
}

func TestValidateThreadDefinitionMutation_WhenOwnerChanges_RejectsChange(t *testing.T) {
	// Arrange
	before := validRepoThreadDefinition()
	after := validRepoThreadDefinition()
	after.Revision = 2
	after.Owner.TaskID = "other-task"

	// Act
	err := validateThreadDefinitionMutation(before, after, nil)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "owner cannot change")
}

func TestValidateThreadDefinitionMutation_WhenStartedLaneKeyChangesCase_RejectsChange(t *testing.T) {
	// Arrange
	before := validRepoThreadDefinition()
	after := validRepoThreadDefinition()
	after.Revision = 2
	after.Threads[0].Key = "Agent-A"
	history := map[string]bool{"f01": true}

	// Act
	err := validateThreadDefinitionMutation(before, after, history)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "key is pinned")
}

func validRepoThreadDefinition() threadDefinition {
	return threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner: threadDefinitionOwner{
			Kind:   threadOwnerTask,
			TaskID: "threaded-task",
		},
		Threads: []threadDefinitionLane{
			{
				Key:     "agent-a",
				Targets: []threadTargetReference{{Target: "F01"}},
			},
			{
				Key:     "both-a",
				Targets: []threadTargetReference{{Target: "F02"}},
				After:   []string{"agent-a"},
			},
		},
	}
}

func validThreadTaskManifest() taskManifest {
	return taskManifest{
		TaskID: "threaded-task",
		Series: "F",
		Artifacts: taskArtifactPaths{
			Slices: []taskSliceArtifact{
				{ID: "F01", Title: "First"},
				{ID: "F02", Title: "Second"},
				{ID: "F03", Title: "Third"},
			},
		},
	}
}

func validWorkspaceThreadDefinition() threadDefinition {
	return threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner: threadDefinitionOwner{
			Kind:        threadOwnerWorkspaceChange,
			WorkspaceID: "umbrella",
			ChangeID:    "change-1",
		},
		Threads: []threadDefinitionLane{
			{
				Key: "parallel",
				Targets: []threadTargetReference{
					{Repo: "api", Task: "api-task", Target: "A01"},
					{Repo: "web", Task: "web-task", Target: "W01"},
				},
			},
		},
	}
}

func validWorkspaceThreadValidationContext() workspaceThreadValidationContext {
	apiManifest := taskManifest{
		TaskID:       "api-task",
		Series:       "A",
		WorkspaceID:  "umbrella",
		ParentChange: "change-1",
		RepoAlias:    "api",
		Artifacts: taskArtifactPaths{
			Slices: []taskSliceArtifact{{ID: "A01", Title: "API"}},
		},
	}
	webManifest := taskManifest{
		TaskID:       "web-task",
		Series:       "W",
		WorkspaceID:  "umbrella",
		ParentChange: "change-1",
		RepoAlias:    "web",
		Artifacts: taskArtifactPaths{
			Slices: []taskSliceArtifact{{ID: "W01", Title: "Web"}},
		},
	}
	return workspaceThreadValidationContext{
		Manifest: workspaceManifest{
			ID: "umbrella",
			Repos: map[string]workspaceRepo{
				"api": {Path: "repos/api"},
				"web": {Path: "repos/web"},
			},
		},
		Change: workspaceChangeFrontmatter{ID: "change-1", Workspace: "umbrella"},
		Links: []workspaceChangeRepoSlice{
			{RepoAlias: "api", TaskID: "api-task", Target: "A01"},
			{RepoAlias: "web", TaskID: "web-task", Target: "W01"},
		},
		TaskManifests: map[string]taskManifest{
			workspaceThreadTaskKey("api", "api-task"): apiManifest,
			workspaceThreadTaskKey("web", "web-task"): webManifest,
		},
		RepoDefinitionExists: map[string]bool{},
	}
}
