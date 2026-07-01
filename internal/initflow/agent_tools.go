package initflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
)

// AgentTool describes one agent/editor surface DevSpecs can prepare command
// files for. J01 detects and selects these; J02 owns actual file generation.
type AgentTool struct {
	ID          string
	Label       string
	Description string
	Detected    bool
	Evidence    []string
	Planned     []string
}

// AgentToolFile describes one generated adapter file.
type AgentToolFile struct {
	ToolID     string
	ToolLabel  string
	Path       string
	Status     string
	Invocation string
}

type agentToolDefinition struct {
	ID          string
	Label       string
	Description string
	DetectPaths []string
	Planned     []string
}

type agentToolFileSpec struct {
	RelPath    string
	Invocation string
	Content    string
}

var agentToolDefinitions = []agentToolDefinition{
	{
		ID:          "codex",
		Label:       "Codex",
		Description: "Codex slash/skill entry points",
		DetectPaths: []string{".agents/skills", ".codex", ".codex/skills", "AGENTS.md"},
		Planned:     []string{"Codex command or skill files for ds task/apply"},
	},
	{
		ID:          "cursor",
		Label:       "Cursor",
		Description: "Cursor command/rule entry points",
		DetectPaths: []string{".cursor", ".cursor/rules", ".cursor/plans", ".cursorrules"},
		Planned:     []string{"Cursor command files for ds task/apply"},
	},
	{
		ID:          "claude",
		Label:       "Claude",
		Description: "Claude slash command and project guidance entry points",
		DetectPaths: []string{".claude", ".claude/commands", ".claude/skills", "CLAUDE.md"},
		Planned:     []string{"Claude command or skill files for ds task/apply"},
	},
	{
		ID:          "windsurf",
		Label:       "Windsurf",
		Description: "Windsurf rule or workflow entry points",
		DetectPaths: []string{".windsurf", ".windsurf/rules", ".windsurfrules"},
		Planned:     []string{"Windsurf rule or command files for ds task/apply"},
	},
}

// DetectAgentTools returns the known agent tooling surfaces, annotated with
// repo-local evidence where present.
func DetectAgentTools(repoRoot string) []AgentTool {
	tools := make([]AgentTool, 0, len(agentToolDefinitions))
	for _, def := range agentToolDefinitions {
		tool := AgentTool{
			ID:          def.ID,
			Label:       def.Label,
			Description: def.Description,
			Planned:     append([]string(nil), def.Planned...),
		}
		for _, rel := range def.DetectPaths {
			if pathExists(filepath.Join(repoRoot, filepath.FromSlash(rel))) {
				tool.Detected = true
				tool.Evidence = append(tool.Evidence, rel)
			}
		}
		tools = append(tools, tool)
	}
	return tools
}

// SelectAgentTools resolves CLI-provided tooling selections. Empty selections
// mean "detected only"; "auto" also means detected only; "all" selects all.
func SelectAgentTools(repoRoot string, selections []string, skip bool) ([]AgentTool, error) {
	tools := DetectAgentTools(repoRoot)
	if skip {
		return nil, nil
	}
	requested := normalizeAgentToolSelections(selections)
	if len(requested) == 0 || hasAgentToolSelection(requested, "auto") {
		return filterAgentTools(tools, func(tool AgentTool) bool { return tool.Detected }), nil
	}
	if hasAgentToolSelection(requested, "none") {
		if len(requested) > 1 {
			return nil, fmt.Errorf("--tool none cannot be combined with other --tool values")
		}
		return nil, nil
	}
	if hasAgentToolSelection(requested, "all") {
		return tools, nil
	}

	byID := make(map[string]AgentTool, len(tools))
	for _, tool := range tools {
		byID[tool.ID] = tool
	}
	var selected []AgentTool
	for _, id := range requested {
		tool, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown agent tooling %q (use codex, cursor, claude, windsurf, auto, all, or none)", id)
		}
		selected = append(selected, tool)
	}
	return selected, nil
}

// RunAgentToolPick lets interactive users review detected agent tooling. The
// caller decides whether to persist or generate anything for selected tools.
func RunAgentToolPick(repoRoot string) ([]AgentTool, error) {
	tools := DetectAgentTools(repoRoot)
	var selected []string
	opts := make([]huh.Option[string], 0, len(tools))
	for _, tool := range tools {
		label := tool.Label
		if tool.Detected {
			label += " (detected: " + strings.Join(tool.Evidence, ", ") + ")"
			selected = append(selected, tool.ID)
		}
		opts = append(opts, huh.NewOption(label, tool.ID))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select agent tooling to prepare").
				Description("Detected tools are pre-selected. DevSpecs will write small command or skill files for the selected tools.").
				Options(opts...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, err
	}
	return selectAgentToolsByID(tools, selected), nil
}

func normalizeAgentToolSelections(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			id := strings.ToLower(strings.TrimSpace(part))
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func selectAgentToolsByID(tools []AgentTool, ids []string) []AgentTool {
	want := map[string]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	return filterAgentTools(tools, func(tool AgentTool) bool {
		_, ok := want[tool.ID]
		return ok
	})
}

func filterAgentTools(tools []AgentTool, keep func(AgentTool) bool) []AgentTool {
	var out []AgentTool
	for _, tool := range tools {
		if keep(tool) {
			out = append(out, tool)
		}
	}
	return out
}

func hasAgentToolSelection(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GenerateAgentToolFiles writes deterministic adapter files for selected tools.
// Existing non-matching files are left untouched unless force is true.
func GenerateAgentToolFiles(repoRoot string, tools []AgentTool, force bool) ([]AgentToolFile, error) {
	var files []AgentToolFile
	for _, tool := range tools {
		for _, spec := range agentToolFileSpecs(tool) {
			status, err := writeAgentToolFile(repoRoot, spec, force)
			if err != nil {
				return files, err
			}
			files = append(files, AgentToolFile{
				ToolID:     tool.ID,
				ToolLabel:  tool.Label,
				Path:       spec.RelPath,
				Status:     status,
				Invocation: spec.Invocation,
			})
		}
	}
	return files, nil
}

func writeAgentToolFile(repoRoot string, spec agentToolFileSpec, force bool) (string, error) {
	rel := filepath.ToSlash(strings.TrimSpace(spec.RelPath))
	if rel == "" {
		return "", fmt.Errorf("empty agent tool path")
	}
	target := filepath.Join(repoRoot, filepath.FromSlash(rel))
	content := normalizeGeneratedContent(spec.Content)
	if existing, err := os.ReadFile(target); err == nil {
		if string(existing) == content {
			return "unchanged", nil
		}
		if !force {
			return "skipped-existing", nil
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", rel, err)
		}
		return "overwritten", nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", rel, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.ToSlash(filepath.Dir(target)), err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", rel, err)
	}
	return "created", nil
}

func normalizeGeneratedContent(content string) string {
	return strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n")) + "\n"
}

func agentToolFileSpecs(tool AgentTool) []agentToolFileSpec {
	switch tool.ID {
	case "codex":
		return []agentToolFileSpec{
			{
				RelPath:    ".agents/skills/ds-task/SKILL.md",
				Invocation: "$ds-task",
				Content:    codexSkillContent("ds-task", "Start or continue one bounded DevSpecs task from the user's goal.", taskAdapterBody("the user's requested goal", "ds task")),
			},
			{
				RelPath:    ".agents/skills/ds-apply/SKILL.md",
				Invocation: "$ds-apply",
				Content:    codexSkillContent("ds-apply", "Apply exactly one DevSpecs task slice or the next available slice with decision gates.", applyAdapterBody("the user's requested target", "ds apply")),
			},
		}
	case "cursor":
		return []agentToolFileSpec{
			{
				RelPath:    ".cursor/commands/ds-task.md",
				Invocation: "/ds-task",
				Content:    slashCommandContent("ds-task", "Start or continue one bounded DevSpecs task from the user's goal.", taskAdapterBody("the user's command arguments", "ds task")),
			},
			{
				RelPath:    ".cursor/commands/ds-apply.md",
				Invocation: "/ds-apply",
				Content:    slashCommandContent("ds-apply", "Apply exactly one DevSpecs task slice or the next available slice with decision gates.", applyAdapterBody("the user's command arguments", "ds apply")),
			},
		}
	case "claude":
		return []agentToolFileSpec{
			{
				RelPath:    ".claude/skills/ds-task/SKILL.md",
				Invocation: "/ds-task",
				Content:    claudeSkillContent("ds-task", "Start or continue one bounded DevSpecs task from the user's goal.", taskAdapterBody("the user's slash-command arguments", "ds task")),
			},
			{
				RelPath:    ".claude/skills/ds-apply/SKILL.md",
				Invocation: "/ds-apply",
				Content:    claudeSkillContent("ds-apply", "Apply exactly one DevSpecs task slice or the next available slice with decision gates.", applyAdapterBody("the user's slash-command arguments", "ds apply")),
			},
		}
	case "windsurf":
		return []agentToolFileSpec{
			{
				RelPath:    ".windsurf/workflows/ds-task.md",
				Invocation: "/ds-task",
				Content:    slashCommandContent("ds-task", "Start or continue one bounded DevSpecs task from the user's goal.", taskAdapterBody("the user's workflow arguments", "ds task")),
			},
			{
				RelPath:    ".windsurf/workflows/ds-apply.md",
				Invocation: "/ds-apply",
				Content:    slashCommandContent("ds-apply", "Apply exactly one DevSpecs task slice or the next available slice with decision gates.", applyAdapterBody("the user's workflow arguments", "ds apply")),
			},
		}
	default:
		return nil
	}
}

func codexSkillContent(name, description, body string) string {
	return fmt.Sprintf(`---
name: %s
description: %s
---

# DevSpecs %s

%s`, name, description, name, body)
}

func claudeSkillContent(name, description, body string) string {
	return fmt.Sprintf(`---
name: %s
description: %s
---

# DevSpecs %s

%s`, name, description, name, body)
}

func slashCommandContent(name, description, body string) string {
	return fmt.Sprintf(`# %s

%s

%s`, name, description, body)
}

func taskAdapterBody(goalPhrase, taskCommand string) string {
	return fmt.Sprintf("Use this adapter when the user wants to start or continue a DevSpecs task.\n\n"+
		"1. Treat %s as the bounded work goal.\n"+
		"2. Prefer `%s \"<bounded-goal>\"` for known work. Add `--quick` only for a tiny one-off.\n"+
		"3. If a task or slice already exists, run `ds apply`, `ds apply <task-id>`, or `ds apply <target>` instead of creating a duplicate task.\n"+
		"4. If the target is unclear, run `ds recent` and `ds find \"<topic>\"` as diagnostics, then return to one bounded task.\n"+
		"5. Work exactly one slice at a time. Do not implement an entire track when the current target is a slice like A01.\n"+
		"6. End with a DevSpecs decision gate: `promote`, `improve`, `rework`, `rollback`, or `block`.\n"+
		"7. Record evidence with `ds task checkpoint <task-id|target> --stage validated --decision <gate>` before claiming the slice is done.\n\n"+
		"Keep `M00` or `A00` as the index, `M01`/`A01` as planned slices, and `M01-1`/`A01-1` as improvement iterations.", goalPhrase, taskCommand)
}

func applyAdapterBody(targetPhrase, applyCommand string) string {
	return fmt.Sprintf("Use this adapter when the user asks to apply the next DevSpecs slice or a specific task target.\n\n"+
		"1. Resolve %s. If no target is provided, let `%s` choose the unambiguous next slice.\n"+
		"2. Run `%s` or `%s <target>`, then follow the emitted DevSpecs prompt exactly.\n"+
		"3. If the target is unclear, run `ds recent` and `ds find \"<topic>\"` as diagnostics, then rerun `ds apply` with one target.\n"+
		"4. Implement only the resolved slice. Do not continue into sibling slices unless the decision gate explicitly promotes to them.\n"+
		"5. Record what changed, files read/edited, tests run, misses, noise, and the next gate using `ds task checkpoint`.\n"+
		"6. Stop after the decision gate. Recommend `promote`, `improve`, `rework`, `rollback`, or `block`.\n\n"+
		"The adapter is a thin wrapper over the DevSpecs CLI. Do not invent a separate task system.", targetPhrase, applyCommand, applyCommand, applyCommand)
}
