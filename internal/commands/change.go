package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type changeCreateOptions struct {
	Workspace string
	Repos     []string
	AsJSON    bool
}

type workspaceChangeFrontmatter struct {
	ID            string   `yaml:"id" json:"id"`
	Type          string   `yaml:"type" json:"type"`
	Workspace     string   `yaml:"workspace" json:"workspace"`
	Status        string   `yaml:"status" json:"status"`
	Title         string   `yaml:"title" json:"title"`
	RequiredRepos []string `yaml:"required_repos" json:"required_repos"`
	OptionalRepos []string `yaml:"optional_repos" json:"optional_repos"`
}

type workspaceChangeRepoSlice struct {
	RepoAlias string
	TaskID    string
	Target    string
	Name      string
	Status    string
}

type changeCreateOutput struct {
	ChangeID      string   `json:"change_id"`
	Title         string   `json:"title"`
	WorkspaceRoot string   `json:"workspace_root"`
	ChangePath    string   `json:"change_path"`
	RequiredRepos []string `json:"required_repos"`
	IndexStatus   string   `json:"index_status"`
	IndexReason   string   `json:"index_reason,omitempty"`
}

// NewChangeCmd creates the ds change command group.
func NewChangeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change",
		Short: "Manage workspace-level change artifacts",
	}
	cmd.AddCommand(newChangeCreateCmd())
	return cmd
}

func newChangeCreateCmd() *cobra.Command {
	var opts changeCreateOptions
	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a workspace-level change artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			err := runChangeCreate(cmd, args[0], opts)
			telemetry.RecordCommand("change_create", err == nil, time.Since(start), map[string]any{
				"json":       opts.AsJSON,
				"repo_count": len(opts.Repos),
				"workspace":  opts.Workspace != "",
			})
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Workspace, "workspace", "", "Workspace root path")
	cmd.Flags().StringSliceVar(&opts.Repos, "repos", nil, "Required workspace repo aliases, comma-separated")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runChangeCreate(cmd *cobra.Command, title string, opts changeCreateOptions) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("change title is empty")
	}
	root, err := resolveWorkspaceRoot(opts.Workspace)
	if err != nil {
		return err
	}
	manifest, err := readWorkspaceManifest(root)
	if err != nil {
		return err
	}
	requiredRepos, err := validateWorkspaceRepoAliases(manifest, opts.Repos)
	if err != nil {
		return err
	}
	changeID, err := nextWorkspaceChangeID(root, manifest)
	if err != nil {
		return err
	}
	changeDir := workspaceChangesDir(root, manifest)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		return fmt.Errorf("create workspace changes dir: %w", err)
	}
	slug := sanitizeTaskFilename(title)
	if slug == "" {
		slug = "change"
	}
	changePath := filepath.Join(changeDir, changeID+"-"+slug+".md")
	if _, err := os.Stat(changePath); err == nil {
		return fmt.Errorf("workspace change already exists: %s", changePath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	frontmatter := workspaceChangeFrontmatter{
		ID:            changeID,
		Type:          "workspace_change",
		Workspace:     manifest.ID,
		Status:        "planned",
		Title:         title,
		RequiredRepos: requiredRepos,
		OptionalRepos: []string{},
	}
	body, err := renderWorkspaceChange(frontmatter, manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(changePath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write workspace change: %w", err)
	}
	out := changeCreateOutput{
		ChangeID:      changeID,
		Title:         title,
		WorkspaceRoot: root,
		ChangePath:    changePath,
		RequiredRepos: requiredRepos,
		IndexStatus:   workspaceIndexStatus,
		IndexReason:   workspaceIndexReason,
	}
	return writeChangeCreateOutput(cmd, out, opts.AsJSON)
}

func renderWorkspaceChange(frontmatter workspaceChangeFrontmatter, manifest workspaceManifest) (string, error) {
	fm, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintln(&b, "---")
	b.Write(fm)
	fmt.Fprintln(&b, "---")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "# %s\n\n", frontmatter.Title)
	fmt.Fprintln(&b, "## Workspace Change")
	fmt.Fprintf(&b, "- ID: `%s`\n", frontmatter.ID)
	fmt.Fprintf(&b, "- Status: `%s`\n", frontmatter.Status)
	fmt.Fprintf(&b, "- Workspace: `%s`\n", frontmatter.Workspace)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Required Repositories")
	for _, alias := range frontmatter.RequiredRepos {
		repo := manifest.Repos[alias]
		fmt.Fprintf(&b, "- `%s` - `%s`", alias, repo.Path)
		if repo.Responsibility != "" {
			fmt.Fprintf(&b, " - %s", repo.Responsibility)
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Notes")
	fmt.Fprintln(&b, "-")
	return b.String(), nil
}

func findWorkspaceChange(root string, manifest workspaceManifest, changeID string) (string, workspaceChangeFrontmatter, string, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return "", workspaceChangeFrontmatter{}, "", fmt.Errorf("change id is empty")
	}
	entries, err := os.ReadDir(workspaceChangesDir(root, manifest))
	if err != nil {
		return "", workspaceChangeFrontmatter{}, "", err
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(entry.Name()), strings.ToUpper(changeID)+"-") && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			matches = append(matches, filepath.Join(workspaceChangesDir(root, manifest), entry.Name()))
		}
	}
	switch len(matches) {
	case 0:
		return "", workspaceChangeFrontmatter{}, "", fmt.Errorf("workspace change %q not found", changeID)
	case 1:
	default:
		return "", workspaceChangeFrontmatter{}, "", fmt.Errorf("workspace change %q is ambiguous", changeID)
	}
	frontmatter, body, err := readWorkspaceChange(matches[0])
	return matches[0], frontmatter, body, err
}

func readWorkspaceChange(path string) (workspaceChangeFrontmatter, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return workspaceChangeFrontmatter{}, "", err
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return workspaceChangeFrontmatter{}, text, fmt.Errorf("workspace change missing frontmatter: %s", path)
	}
	rest := strings.TrimPrefix(text, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return workspaceChangeFrontmatter{}, text, fmt.Errorf("workspace change frontmatter is not closed: %s", path)
	}
	var frontmatter workspaceChangeFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &frontmatter); err != nil {
		return workspaceChangeFrontmatter{}, text, fmt.Errorf("parse workspace change frontmatter: %w", err)
	}
	return frontmatter, text, nil
}

func upsertWorkspaceChangeRepoSlice(path string, link workspaceChangeRepoSlice) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	body := string(data)
	updated := upsertWorkspaceChangeRepoSliceBody(body, link)
	return os.WriteFile(path, []byte(updated), 0o644)
}

func upsertWorkspaceChangeRepoSliceBody(body string, link workspaceChangeRepoSlice) string {
	row := renderWorkspaceChangeRepoSliceRow(link)
	header := "## Repo Slices\n| Repo | Task ID | Target | Name | Status |\n| --- | --- | --- | --- | --- |\n"
	start := strings.Index(body, "\n## Repo Slices\n")
	if start < 0 {
		section := "\n" + header + row + "\n"
		if notes := strings.Index(body, "\n## Notes"); notes >= 0 {
			return body[:notes] + section + body[notes:]
		}
		if strings.HasSuffix(body, "\n") {
			return body + section
		}
		return body + "\n" + section
	}
	sectionStart := start + 1
	sectionBodyStart := sectionStart + len(header)
	if sectionBodyStart > len(body) || body[sectionStart:sectionStart+len("## Repo Slices\n")] != "## Repo Slices\n" {
		return body + "\n" + header + row
	}
	next := strings.Index(body[sectionBodyStart:], "\n## ")
	sectionEnd := len(body)
	if next >= 0 {
		sectionEnd = sectionBodyStart + next
	}
	lines := strings.Split(body[sectionBodyStart:sectionEnd], "\n")
	replaced := false
	for i, line := range lines {
		if strings.Contains(line, "| `"+link.RepoAlias+"` |") {
			lines[i] = strings.TrimRight(row, "\n")
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append([]string{strings.TrimRight(row, "\n")}, lines...)
	}
	rebuilt := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if rebuilt != "" {
		rebuilt += "\n"
	}
	return body[:sectionBodyStart] + rebuilt + body[sectionEnd:]
}

func renderWorkspaceChangeRepoSliceRow(link workspaceChangeRepoSlice) string {
	status := strings.TrimSpace(link.Status)
	if status == "" {
		status = "planned"
	}
	return fmt.Sprintf("| `%s` | `%s` | `%s` | %s | `%s` |\n",
		escapeMarkdownTableCell(link.RepoAlias),
		escapeMarkdownTableCell(link.TaskID),
		escapeMarkdownTableCell(link.Target),
		escapeMarkdownTableCell(link.Name),
		escapeMarkdownTableCell(status),
	)
}

func parseWorkspaceChangeRepoSlices(body string) []workspaceChangeRepoSlice {
	start := strings.Index(body, "\n## Repo Slices\n")
	if start < 0 {
		return nil
	}
	sectionStart := start + len("\n## Repo Slices\n")
	next := strings.Index(body[sectionStart:], "\n## ")
	sectionEnd := len(body)
	if next >= 0 {
		sectionEnd = sectionStart + next
	}
	var rows []workspaceChangeRepoSlice
	for _, line := range strings.Split(body[sectionStart:sectionEnd], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "|") || strings.Contains(line, "---") || strings.Contains(line, "Repo | Task ID") {
			continue
		}
		cells := markdownTableCells(line)
		if len(cells) < 5 {
			continue
		}
		rows = append(rows, workspaceChangeRepoSlice{
			RepoAlias: stripMarkdownCellCode(cells[0]),
			TaskID:    stripMarkdownCellCode(cells[1]),
			Target:    stripMarkdownCellCode(cells[2]),
			Name:      stripMarkdownCellCode(cells[3]),
			Status:    stripMarkdownCellCode(cells[4]),
		})
	}
	return rows
}

func markdownTableCells(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(strings.ReplaceAll(part, `\|`, "|")))
	}
	return cells
}

func stripMarkdownCellCode(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`")
	return strings.TrimSpace(value)
}

func writeChangeCreateOutput(cmd *cobra.Command, out changeCreateOutput, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created workspace change: %s\n", out.ChangeID)
	fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", out.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "Path: %s\n", out.ChangePath)
	fmt.Fprintf(cmd.OutOrStdout(), "Repos: %s\n", strings.Join(out.RequiredRepos, ", "))
	if out.IndexStatus != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Index: %s", out.IndexStatus)
		if out.IndexReason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", out.IndexReason)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	return nil
}

func nextWorkspaceChangeID(root string, manifest workspaceManifest) (string, error) {
	prefix := workspaceChangePrefix(manifest.ID)
	highest := 0
	entries, err := os.ReadDir(workspaceChangesDir(root, manifest))
	if err != nil {
		if os.IsNotExist(err) {
			return prefix + "-C001", nil
		}
		return "", err
	}
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `-C([0-9]{3,})\b`)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := pattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err == nil && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("%s-C%03d", prefix, highest+1), nil
}

func workspaceChangePrefix(workspaceID string) string {
	workspaceID = sanitizeWorkspaceID(workspaceID)
	if workspaceID == "" {
		return "WS"
	}
	first, _, _ := strings.Cut(workspaceID, "-")
	first = strings.TrimSpace(first)
	if first == "" {
		return "WS"
	}
	return strings.ToUpper(first)
}
