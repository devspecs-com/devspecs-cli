package commands

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

type sliceCreateOptions struct {
	Workspace string
	Repo      string
	RepoPath  string
	Name      string
	NoRefresh bool
	Index     bool
	AsJSON    bool
}

type sliceCreateOutput struct {
	ChangeID      string `json:"change_id"`
	TaskID        string `json:"task_id"`
	Target        string `json:"target"`
	RepoAlias     string `json:"repo_alias,omitempty"`
	RepoRoot      string `json:"repo_root"`
	WorkspaceRoot string `json:"workspace_root"`
	TaskWorkspace string `json:"task_workspace"`
	PlanPath      string `json:"plan_path"`
	ResultPath    string `json:"result_path"`
	ManifestPath  string `json:"manifest_path"`
	ChangePath    string `json:"change_path"`
}

// NewSliceCmd creates the ds slice command group.
func NewSliceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "slice",
		Short: "Create repo-local task slices from workspace changes",
	}
	cmd.AddCommand(newSliceCreateCmd())
	return cmd
}

func newSliceCreateCmd() *cobra.Command {
	opts := sliceCreateOptions{Index: true}
	cmd := &cobra.Command{
		Use:   "create <change-id>",
		Short: "Create a repo-local task linked to a workspace change",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			err := runSliceCreate(cmd, args[0], opts)
			telemetry.RecordCommand("slice_create", err == nil, time.Since(start), map[string]any{
				"json":       opts.AsJSON,
				"workspace":  opts.Workspace != "",
				"repo_alias": opts.Repo != "",
				"repo_path":  opts.RepoPath != "",
			})
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Workspace, "workspace", "", "Workspace root path")
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "Workspace repo alias or explicit repo path")
	cmd.Flags().StringVar(&opts.RepoPath, "repo-path", "", "Explicit repo path when not using a workspace repo alias")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Repo-local slice name")
	cmd.Flags().BoolVar(&opts.NoRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture created task artifacts into the DevSpecs index")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runSliceCreate(cmd *cobra.Command, changeID string, opts sliceCreateOptions) error {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return fmt.Errorf("change id is empty")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	workspaceRoot, err := resolveWorkspaceRoot(opts.Workspace)
	if err != nil {
		return err
	}
	workspaceManifest, err := readWorkspaceManifest(workspaceRoot)
	if err != nil {
		return err
	}
	changePath, changeFrontmatter, _, err := findWorkspaceChange(workspaceRoot, workspaceManifest, changeID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(changeFrontmatter.ID, changeID) {
		return fmt.Errorf("workspace change id mismatch: %s", changeFrontmatter.ID)
	}
	repoAlias, repoPath, err := resolveSliceRepo(workspaceRoot, workspaceManifest, opts)
	if err != nil {
		return err
	}
	repoRoot, err := resolveTargetRepoRoot(repoPath)
	if err != nil {
		return err
	}

	taskID := workspaceSliceTaskID(changeID, repoAlias, name)
	targetName := name
	query := fmt.Sprintf("%s for workspace change %s: %s", name, changeFrontmatter.ID, changeFrontmatter.Title)
	startOut, _, err := createTaskWorkspace(cmd, query, taskStartOptions{
		ID:        taskID,
		Dir:       defaultTaskWorkspaceDir,
		Repo:      repoRoot,
		Profile:   taskProfileCodeChange,
		Slices:    []string{targetName},
		NoRefresh: opts.NoRefresh,
		Index:     opts.Index,
		WorkspaceLink: taskWorkspaceLink{
			WorkspaceID:   workspaceManifest.ID,
			WorkspaceRoot: workspaceRoot,
			ParentChange:  changeFrontmatter.ID,
			RepoAlias:     repoAlias,
		},
	})
	if err != nil {
		return err
	}
	target := ""
	planPath := startOut.FirstSlicePath
	resultPath := startOut.ResultPath
	if len(startOut.Slices) > 0 {
		target = startOut.Slices[0].ID
		planPath = startOut.Slices[0].PlanPath
		resultPath = startOut.Slices[0].ResultPath
	}
	if target == "" {
		target = defaultTaskSeries(startOut.Series) + "01"
	}
	if err := upsertWorkspaceChangeRepoSlice(changePath, workspaceChangeRepoSlice{
		RepoAlias: repoAlias,
		TaskID:    startOut.TaskID,
		Target:    target,
		Name:      name,
		Status:    "planned",
	}); err != nil {
		return err
	}
	if err := upsertWorkspaceTraceEdge(workspaceTraceEdgeLink{
		WorkspaceRoot: workspaceRoot,
		ChangeID:      changeFrontmatter.ID,
		ChangePath:    changePath,
		TaskID:        startOut.TaskID,
		Target:        target,
		RepoAlias:     repoAlias,
		TaskWorkspace: startOut.Workspace,
		ManifestPath:  startOut.ManifestPath,
		Status:        "planned",
	}); err != nil {
		return err
	}
	out := sliceCreateOutput{
		ChangeID:      changeFrontmatter.ID,
		TaskID:        startOut.TaskID,
		Target:        target,
		RepoAlias:     repoAlias,
		RepoRoot:      repoRoot,
		WorkspaceRoot: workspaceRoot,
		TaskWorkspace: startOut.Workspace,
		PlanPath:      planPath,
		ResultPath:    resultPath,
		ManifestPath:  startOut.ManifestPath,
		ChangePath:    changePath,
	}
	return writeSliceCreateOutput(cmd, out, opts.AsJSON)
}

func resolveSliceRepo(workspaceRoot string, manifest workspaceManifest, opts sliceCreateOptions) (string, string, error) {
	repoArg := strings.TrimSpace(opts.Repo)
	repoPathArg := strings.TrimSpace(opts.RepoPath)
	if repoPathArg != "" {
		alias := ""
		if repoArg != "" {
			if _, ok := manifest.Repos[repoArg]; ok {
				alias = repoArg
			} else if !looksLikePath(repoArg) {
				return "", "", fmt.Errorf("workspace repo alias %q not found in %s", repoArg, workspaceManifestFilename)
			}
		}
		return alias, workspaceRelativeAbs(workspaceRoot, repoPathArg), nil
	}
	if repoArg == "" {
		return "", "", fmt.Errorf("--repo is required")
	}
	if repo, ok := manifest.Repos[repoArg]; ok {
		return repoArg, workspaceRelativeAbs(workspaceRoot, repo.Path), nil
	}
	if !looksLikePath(repoArg) {
		return "", "", fmt.Errorf("workspace repo alias %q not found in %s", repoArg, workspaceManifestFilename)
	}
	return "", workspaceRelativeAbs(workspaceRoot, repoArg), nil
}

func workspaceRelativeAbs(root, path string) string {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, filepath.FromSlash(path)))
}

func looksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	return value == "." ||
		strings.HasPrefix(value, ".") ||
		strings.Contains(value, "/") ||
		strings.Contains(value, `\`) ||
		filepath.IsAbs(value)
}

func workspaceSliceTaskID(changeID, repoAlias, name string) string {
	repoPart := strings.TrimSpace(repoAlias)
	if repoPart == "" {
		repoPart = sanitizeWorkspaceID(name)
	}
	id := sanitizeWorkspaceID(changeID + "-" + repoPart)
	if id == "" {
		id = sanitizeWorkspaceID(changeID + "-" + name)
	}
	if id == "" {
		return generatedTaskID(name, time.Now().UTC())
	}
	return id
}

func writeSliceCreateOutput(cmd *cobra.Command, out sliceCreateOutput, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created workspace slice task: %s\n", out.TaskID)
	fmt.Fprintf(cmd.OutOrStdout(), "Target: %s\n", out.Target)
	if out.RepoAlias != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Repo: %s\n", out.RepoAlias)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Task workspace: %s\n", out.TaskWorkspace)
	fmt.Fprintf(cmd.OutOrStdout(), "Plan: %s\n", out.PlanPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Result: %s\n", out.ResultPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Change: %s\n", out.ChangePath)
	return nil
}
