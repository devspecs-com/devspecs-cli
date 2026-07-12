package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

const (
	traceEdgeWorkspaceChangeHasSlice = "workspace_change_has_slice"
	traceStatusComplete              = "complete"
	traceStatusIncomplete            = "incomplete"
	traceIndexIndexed                = "indexed"
	traceIndexMissing                = "index_missing"
	traceIndexUnknown                = "unknown"
)

type traceOptions struct {
	Workspace string
	Repo      string
	AsJSON    bool
}

type traceOutput struct {
	ID            string             `json:"id"`
	Kind          string             `json:"kind"`
	Status        string             `json:"status,omitempty"`
	WorkspaceRoot string             `json:"workspace_root,omitempty"`
	ChangeID      string             `json:"change_id,omitempty"`
	ChangePath    string             `json:"change_path,omitempty"`
	TaskID        string             `json:"task_id,omitempty"`
	RepoRoot      string             `json:"repo_root,omitempty"`
	ParentChange  string             `json:"parent_change,omitempty"`
	RepoAlias     string             `json:"repo_alias,omitempty"`
	Slices        []traceSliceOutput `json:"slices,omitempty"`
	Edges         []traceEdgeOutput  `json:"edges,omitempty"`
	Notes         []string           `json:"notes,omitempty"`
}

type traceSliceOutput struct {
	RepoAlias     string   `json:"repo_alias,omitempty"`
	TaskID        string   `json:"task_id,omitempty"`
	Target        string   `json:"target,omitempty"`
	Name          string   `json:"name,omitempty"`
	Status        string   `json:"status"`
	Decision      string   `json:"decision,omitempty"`
	IndexStatus   string   `json:"index_status,omitempty"`
	TaskWorkspace string   `json:"task_workspace,omitempty"`
	ManifestPath  string   `json:"manifest_path,omitempty"`
	PlanPath      string   `json:"plan_path,omitempty"`
	ResultPath    string   `json:"result_path,omitempty"`
	RepoRoot      string   `json:"repo_root,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

type traceEdgeOutput struct {
	EdgeType      string `json:"edge_type"`
	SrcArtifactID string `json:"src_artifact_id"`
	DstArtifactID string `json:"dst_artifact_id"`
	MetadataJSON  string `json:"metadata_json,omitempty"`
}

type workspaceTraceEdgeLink struct {
	WorkspaceRoot string
	ChangeID      string
	ChangePath    string
	TaskID        string
	Target        string
	RepoAlias     string
	TaskWorkspace string
	ManifestPath  string
	Status        string
}

// NewTraceCmd creates the ds trace command.
func NewTraceCmd() *cobra.Command {
	var opts traceOptions
	cmd := &cobra.Command{
		Use:   "trace <change-id|task-id>",
		Short: "Trace workspace changes to repo-local task slices",
		Long: `Trace a known workspace change or repo task to linked repo-local slices.

Use ds workspace trace only when you already know the workspace change ID or repo
task ID. Use ds find to discover relevant context first, and use ds task status
for task lifecycle progress. In trace output, status describes change/task
lifecycle while index_status describes local index capture state.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			err := runTrace(cmd, args[0], opts)
			telemetry.RecordCommand("trace", err == nil, time.Since(start), map[string]any{
				"json":      opts.AsJSON,
				"workspace": opts.Workspace != "",
				"repo":      opts.Repo != "",
			})
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Workspace, "workspace", "", "Workspace root path")
	cmd.Flags().StringVar(&opts.Repo, repoTargetFlagName, "", "Target repository path for repo-local task trace")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runTrace(cmd *cobra.Command, id string, opts traceOptions) error {
	out, err := buildTraceOutput(id, opts)
	if err != nil {
		return err
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return writeTraceHuman(cmd, out)
}

func buildTraceOutput(id string, opts traceOptions) (traceOutput, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return traceOutput{}, fmt.Errorf("trace id is empty")
	}
	if strings.TrimSpace(opts.Workspace) != "" {
		if out, err := buildWorkspaceChangeTrace(id, opts.Workspace); err == nil {
			return out, nil
		}
	}
	if strings.TrimSpace(opts.Repo) != "" {
		return buildTaskTrace(id, opts)
	}
	if out, err := buildWorkspaceChangeTrace(id, ""); err == nil {
		return out, nil
	}
	return buildTaskTrace(id, opts)
}

func buildWorkspaceChangeTrace(changeID, workspacePath string) (traceOutput, error) {
	root, err := resolveWorkspaceRoot(workspacePath)
	if err != nil {
		return traceOutput{}, err
	}
	manifest, err := readWorkspaceManifest(root)
	if err != nil {
		return traceOutput{}, err
	}
	changePath, frontmatter, body, err := findWorkspaceChange(root, manifest, changeID)
	if err != nil {
		return traceOutput{}, err
	}
	rows := parseWorkspaceChangeRepoSlices(body)
	out := traceOutput{
		ID:            frontmatter.ID,
		Kind:          "workspace_change",
		WorkspaceRoot: root,
		ChangeID:      frontmatter.ID,
		ChangePath:    changePath,
		Edges:         workspaceTraceEdges(root, frontmatter.ID),
	}
	for _, row := range rows {
		out.Slices = append(out.Slices, buildTraceSliceFromLink(root, manifest, row))
	}
	out.Status, out.Notes = traceWorkspaceChangeStatus(frontmatter, out.Slices, out.Notes)
	if len(out.Slices) == 0 {
		out.Notes = append(out.Notes, "no repo slices linked yet")
	}
	return out, nil
}

func buildTaskTrace(taskID string, opts traceOptions) (traceOutput, error) {
	repoRoot, workspace, manifest, err := loadTaskWorkspaceManifestForRepo(defaultTaskWorkspaceDir, taskID, opts.Repo)
	if err != nil {
		return traceOutput{}, err
	}
	out := traceOutput{
		ID:           manifest.TaskID,
		Kind:         "repo_task",
		TaskID:       manifest.TaskID,
		RepoRoot:     repoRoot,
		ParentChange: manifest.ParentChange,
		RepoAlias:    manifest.RepoAlias,
	}
	target := firstTaskSliceArtifact(manifest.Artifacts)
	if target.ID != "" {
		out.Slices = append(out.Slices, buildTraceSliceFromManifest(repoRoot, workspace, manifest, workspaceChangeRepoSlice{
			RepoAlias: manifest.RepoAlias,
			TaskID:    manifest.TaskID,
			Target:    target.ID,
			Name:      target.Title,
		}))
	}
	workspaceRoot := strings.TrimSpace(firstNonEmptyTaskString(opts.Workspace, manifest.WorkspaceRoot))
	if workspaceRoot == "" || manifest.ParentChange == "" {
		out.Notes = append(out.Notes, "task has no workspace parent change metadata")
		return out, nil
	}
	out.WorkspaceRoot = workspaceRoot
	if changeTrace, err := buildWorkspaceChangeTrace(manifest.ParentChange, workspaceRoot); err == nil {
		out.Status = changeTrace.Status
		out.ChangeID = changeTrace.ChangeID
		out.ChangePath = changeTrace.ChangePath
		out.Edges = changeTrace.Edges
		out.Slices = changeTrace.Slices
	} else {
		out.Notes = append(out.Notes, err.Error())
	}
	return out, nil
}

func traceWorkspaceChangeStatus(frontmatter workspaceChangeFrontmatter, slices []traceSliceOutput, notes []string) (string, []string) {
	if len(frontmatter.RequiredRepos) == 0 {
		if len(slices) == 0 {
			return traceStatusIncomplete, append(notes, "workspace change has no required repo slices")
		}
		for _, slice := range slices {
			if !traceSliceCompleteOrRuledOut(slice.Status) {
				return traceStatusIncomplete, notes
			}
		}
		return traceStatusComplete, notes
	}
	byRepo := make(map[string]traceSliceOutput, len(slices))
	for _, slice := range slices {
		alias := strings.TrimSpace(slice.RepoAlias)
		if alias != "" {
			byRepo[alias] = slice
		}
	}
	complete := true
	for _, alias := range frontmatter.RequiredRepos {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		slice, ok := byRepo[alias]
		if !ok || strings.TrimSpace(slice.TaskID) == "" {
			notes = append(notes, fmt.Sprintf("required repo %s has no linked slice", alias))
			complete = false
			continue
		}
		if !traceSliceCompleteOrRuledOut(slice.Status) {
			complete = false
		}
	}
	if complete {
		return traceStatusComplete, notes
	}
	return traceStatusIncomplete, notes
}

func traceSliceCompleteOrRuledOut(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "blocked", "cancelled", "canceled", "superseded":
		return true
	default:
		return false
	}
}

func buildTraceSliceFromLink(workspaceRoot string, workspaceManifest workspaceManifest, link workspaceChangeRepoSlice) traceSliceOutput {
	row := traceSliceOutput{
		RepoAlias:   strings.TrimSpace(link.RepoAlias),
		TaskID:      strings.TrimSpace(link.TaskID),
		Target:      strings.TrimSpace(link.Target),
		Name:        strings.TrimSpace(link.Name),
		Status:      firstNonEmptyTaskString(strings.TrimSpace(link.Status), "planned"),
		IndexStatus: traceIndexUnknown,
	}
	repo, ok := workspaceManifest.Repos[row.RepoAlias]
	if !ok {
		row.Notes = append(row.Notes, "repo alias not found in workspace manifest")
		return row
	}
	repoRoot := workspaceRelativeAbs(workspaceRoot, repo.Path)
	row.RepoRoot = repoRoot
	_, workspace, manifest, err := loadTaskWorkspaceManifestForRepo(defaultTaskWorkspaceDir, row.TaskID, repoRoot)
	if err != nil {
		row.Notes = append(row.Notes, err.Error())
		return row
	}
	return buildTraceSliceFromManifest(repoRoot, workspace, manifest, link)
}

func buildTraceSliceFromManifest(repoRoot, workspace string, manifest taskManifest, link workspaceChangeRepoSlice) traceSliceOutput {
	row := traceSliceOutput{
		RepoAlias:     firstNonEmptyTaskString(strings.TrimSpace(link.RepoAlias), manifest.RepoAlias),
		TaskID:        manifest.TaskID,
		Target:        strings.TrimSpace(link.Target),
		Name:          strings.TrimSpace(link.Name),
		TaskWorkspace: workspace,
		ManifestPath:  filepath.Join(workspace, taskManifestFilename),
		RepoRoot:      repoRoot,
	}
	if row.Target == "" {
		row.Target = firstTaskSliceArtifact(manifest.Artifacts).ID
	}
	slice, err := taskSliceForCheckpoint(manifest, row.Target)
	if err != nil {
		row.Status = firstNonEmptyTaskString(strings.TrimSpace(link.Status), "planned")
		row.Notes = append(row.Notes, err.Error())
		return row
	}
	if row.Name == "" {
		row.Name = slice.Title
	}
	row.Target = slice.ID
	row.PlanPath = filepath.Join(workspace, filepath.FromSlash(slice.Plan))
	row.ResultPath = filepath.Join(workspace, filepath.FromSlash(slice.Result))
	row.Status, row.Decision = traceTaskStatus(manifest, slice, row.ResultPath)
	row.IndexStatus = traceTaskIndexStatus(repoRoot, row.PlanPath)
	return row
}

func traceTaskStatus(manifest taskManifest, slice taskSliceArtifact, resultPath string) (string, string) {
	if strings.TrimSpace(resultPath) != "" {
		if _, err := os.Stat(resultPath); os.IsNotExist(err) {
			return "missing_result", slice.Decision
		}
	}
	stage, decision := taskStateForTarget(manifest, slice.ID)
	if strings.EqualFold(decision, "block") || strings.EqualFold(decision, "blocked") || strings.EqualFold(stage, "blocked") {
		return "blocked", decision
	}
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "promote", "complete", "completed":
		return "completed", decision
	}
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "completed", "done":
		return "completed", decision
	case "started", "implemented", "validated":
		return "started", decision
	case "", "packed", "planned":
		return "planned", decision
	default:
		return stage, decision
	}
}

func traceTaskIndexStatus(repoRoot, artifactPath string) string {
	rel := taskRelativePath(repoRoot, artifactPath)
	if rel == "" {
		return traceIndexUnknown
	}
	db, err := openDB()
	if err != nil {
		return traceIndexUnknown
	}
	defer db.Close()
	meta := db.GetRepoByRoot(canonicalRepoRoot(repoRoot))
	if meta == nil {
		if _, err := os.Stat(artifactPath); err == nil {
			return traceIndexMissing
		}
		return traceIndexUnknown
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sources WHERE repo_id = ? AND path = ? AND source_type = 'capture'", meta.ID, rel).Scan(&count); err != nil {
		return traceIndexUnknown
	}
	if count > 0 {
		return traceIndexIndexed
	}
	if _, err := os.Stat(artifactPath); err == nil {
		return traceIndexMissing
	}
	return traceIndexUnknown
}

func upsertWorkspaceTraceEdge(link workspaceTraceEdgeLink) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	repoID, err := ensureTaskFactRepo(db, canonicalRepoRoot(link.WorkspaceRoot), now)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]string{
		"workspace_root": link.WorkspaceRoot,
		"change_id":      link.ChangeID,
		"change_path":    link.ChangePath,
		"task_id":        link.TaskID,
		"target":         link.Target,
		"repo_alias":     link.RepoAlias,
		"task_workspace": link.TaskWorkspace,
		"manifest_path":  link.ManifestPath,
		"status":         firstNonEmptyTaskString(link.Status, "planned"),
	})
	if err != nil {
		return err
	}
	return db.UpsertArtifactEdge(store.ArtifactEdgeInput{
		ID:            traceEdgeID(link.ChangeID, link.TaskID, link.RepoAlias),
		RepoID:        repoID,
		SrcArtifactID: traceChangeArtifactID(link.ChangeID),
		DstArtifactID: traceTaskArtifactID(link.TaskID),
		EdgeType:      traceEdgeWorkspaceChangeHasSlice,
		Weight:        1,
		Confidence:    1,
		EvidenceCount: 1,
		Freshness:     "current",
		SourceSignal:  "workspace_change_repo_slice",
		Explanation:   "workspace change links to repo-local task slice",
		MetadataJSON:  string(metadata),
	}, now)
}

func workspaceTraceEdges(workspaceRoot, changeID string) []traceEdgeOutput {
	db, err := openDB()
	if err != nil {
		return nil
	}
	defer db.Close()
	meta := db.GetRepoByRoot(canonicalRepoRoot(workspaceRoot))
	if meta == nil {
		return nil
	}
	edges, err := db.GetArtifactEdges(store.ArtifactEdgeFilter{
		RepoID:        meta.ID,
		SrcArtifactID: traceChangeArtifactID(changeID),
		EdgeType:      traceEdgeWorkspaceChangeHasSlice,
	})
	if err != nil {
		return nil
	}
	var out []traceEdgeOutput
	for _, edge := range edges {
		out = append(out, traceEdgeOutput{
			EdgeType:      edge.EdgeType,
			SrcArtifactID: edge.SrcArtifactID,
			DstArtifactID: edge.DstArtifactID,
			MetadataJSON:  edge.MetadataJSON,
		})
	}
	return out
}

func traceChangeArtifactID(changeID string) string {
	return "workspace_change:" + strings.TrimSpace(changeID)
}

func traceTaskArtifactID(taskID string) string {
	return "task:" + strings.TrimSpace(taskID)
}

func traceEdgeID(changeID, taskID, repoAlias string) string {
	return "edge_" + sanitizeWorkspaceID(changeID+"-"+repoAlias+"-"+taskID)
}

func writeTraceHuman(cmd *cobra.Command, out traceOutput) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Trace: %s (%s)\n", out.ID, out.Kind)
	if out.WorkspaceRoot != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Workspace: %s\n", out.WorkspaceRoot)
	}
	if out.ChangeID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Change: %s\n", out.ChangeID)
	}
	if out.Status != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", out.Status)
	}
	if out.ParentChange != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Parent Change: %s\n", out.ParentChange)
	}
	if len(out.Slices) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Slices:")
		for _, slice := range out.Slices {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s %s %s status=%s index=%s", emptyAsDash(slice.RepoAlias), emptyAsDash(slice.TaskID), emptyAsDash(slice.Target), slice.Status, emptyAsDash(slice.IndexStatus))
			if slice.Decision != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " decision=%s", slice.Decision)
			}
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	for _, note := range out.Notes {
		fmt.Fprintf(cmd.OutOrStdout(), "Note: %s\n", note)
	}
	return nil
}
