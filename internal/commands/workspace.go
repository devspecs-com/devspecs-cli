package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

type workspaceInitOptions struct {
	AsJSON bool
}

type workspaceShowOptions struct {
	Workspace string
	AsJSON    bool
}

type workspaceOutput struct {
	WorkspaceRoot string            `json:"workspace_root"`
	ManifestPath  string            `json:"manifest_path"`
	DocumentPath  string            `json:"document_path"`
	ChangesDir    string            `json:"changes_dir"`
	Manifest      workspaceManifest `json:"manifest"`
	IndexStatus   string            `json:"index_status"`
	IndexReason   string            `json:"index_reason,omitempty"`
}

// NewWorkspaceCmd creates the ds workspace command group.
func NewWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"ws"},
		Short:   "Manage workspace-level DevSpecs artifacts",
	}
	cmd.AddCommand(newWorkspaceInitCmd())
	cmd.AddCommand(newWorkspaceShowCmd())
	cmd.AddCommand(NewChangeCmd())
	cmd.AddCommand(NewSliceCmd())
	cmd.AddCommand(NewTraceCmd())
	return cmd
}

func newWorkspaceInitCmd() *cobra.Command {
	var opts workspaceInitOptions
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Create workspace-level DevSpecs artifacts",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			err := runWorkspaceInit(cmd, path, opts)
			telemetry.RecordCommand("workspace_init", err == nil, time.Since(start), map[string]any{
				"json": opts.AsJSON,
			})
			return err
		},
	}
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newWorkspaceShowCmd() *cobra.Command {
	var opts workspaceShowOptions
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the resolved DevSpecs workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			err := runWorkspaceShow(cmd, opts)
			telemetry.RecordCommand("workspace_show", err == nil, time.Since(start), map[string]any{
				"json":      opts.AsJSON,
				"workspace": opts.Workspace != "",
			})
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Workspace, "workspace", "", "Workspace root path")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runWorkspaceInit(cmd *cobra.Command, path string, opts workspaceInitOptions) error {
	root, err := resolveWorkspaceRootForInit(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, defaultWorkspaceArtifactDir), 0o755); err != nil {
		return fmt.Errorf("create workspace artifact dir: %w", err)
	}
	manifestPath := workspaceManifestPath(root)
	documentPath := workspaceDocumentPath(root)
	if _, err := os.Stat(manifestPath); err == nil {
		return fmt.Errorf("workspace already initialized: %s", manifestPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(documentPath); err == nil {
		return fmt.Errorf("workspace document already exists: %s", documentPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	manifest, err := newWorkspaceManifest(root)
	if err != nil {
		return err
	}
	changesDir := workspaceChangesDir(root, manifest)
	if err := os.MkdirAll(changesDir, 0o755); err != nil {
		return fmt.Errorf("create workspace changes dir: %w", err)
	}
	if err := writeWorkspaceManifest(manifestPath, manifest); err != nil {
		return fmt.Errorf("write workspace manifest: %w", err)
	}
	if err := os.WriteFile(documentPath, []byte(renderWorkspaceDocument(manifest)), 0o644); err != nil {
		return fmt.Errorf("write workspace document: %w", err)
	}
	out := workspaceOutput{
		WorkspaceRoot: root,
		ManifestPath:  manifestPath,
		DocumentPath:  documentPath,
		ChangesDir:    changesDir,
		Manifest:      manifest,
		IndexStatus:   workspaceIndexStatus,
		IndexReason:   workspaceIndexReason,
	}
	return writeWorkspaceOutput(cmd, out, opts.AsJSON)
}

func runWorkspaceShow(cmd *cobra.Command, opts workspaceShowOptions) error {
	root, err := resolveWorkspaceRoot(opts.Workspace)
	if err != nil {
		return err
	}
	manifest, err := readWorkspaceManifest(root)
	if err != nil {
		return err
	}
	out := workspaceOutput{
		WorkspaceRoot: root,
		ManifestPath:  workspaceManifestPath(root),
		DocumentPath:  workspaceDocumentPath(root),
		ChangesDir:    workspaceChangesDir(root, manifest),
		Manifest:      manifest,
		IndexStatus:   workspaceIndexStatus,
		IndexReason:   workspaceIndexReason,
	}
	return writeWorkspaceOutput(cmd, out, opts.AsJSON)
}

func writeWorkspaceOutput(cmd *cobra.Command, out workspaceOutput, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Workspace: %s\n", out.WorkspaceRoot)
	fmt.Fprintf(cmd.OutOrStdout(), "Manifest: %s\n", out.ManifestPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Document: %s\n", out.DocumentPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Changes: %s\n", out.ChangesDir)
	fmt.Fprintf(cmd.OutOrStdout(), "ID: %s\n", out.Manifest.ID)
	if len(out.Manifest.Repos) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Repos: none")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Repos:")
		for _, alias := range sortedWorkspaceRepoAliases(out.Manifest.Repos) {
			repo := out.Manifest.Repos[alias]
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", alias, repo.Path)
		}
	}
	if out.IndexStatus != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Index: %s", out.IndexStatus)
		if out.IndexReason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", out.IndexReason)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	return nil
}

func renderWorkspaceDocument(manifest workspaceManifest) string {
	manifest = normalizeWorkspaceManifest(manifest)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s Workspace\n\n", manifest.Name)
	fmt.Fprintf(&b, "- ID: `%s`\n", manifest.ID)
	fmt.Fprintf(&b, "- Artifact dir: `%s`\n", manifest.ArtifactDir)
	fmt.Fprintf(&b, "- Changes: `%s/%s/`\n\n", manifest.ArtifactDir, workspaceChangesDirName)
	fmt.Fprintln(&b, "## Repositories")
	if len(manifest.Repos) == 0 {
		fmt.Fprintln(&b, "- No repositories registered yet.")
	} else {
		fmt.Fprintln(&b, "| Alias | Path | Responsibility |")
		fmt.Fprintln(&b, "| --- | --- | --- |")
		for _, alias := range sortedWorkspaceRepoAliases(manifest.Repos) {
			repo := manifest.Repos[alias]
			fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", escapeMarkdownTableCell(alias), escapeMarkdownTableCell(repo.Path), escapeMarkdownTableCell(repo.Responsibility))
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Workspace Changes")
	fmt.Fprintf(&b, "- Store cross-repo change artifacts in `%s/%s/`.\n", manifest.ArtifactDir, workspaceChangesDirName)
	fmt.Fprintln(&b, "- Keep repo-local execution artifacts inside each target repository.")
	return b.String()
}
