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

type agentToolDefinition struct {
	ID          string
	Label       string
	Description string
	DetectPaths []string
	Planned     []string
}

var agentToolDefinitions = []agentToolDefinition{
	{
		ID:          "codex",
		Label:       "Codex",
		Description: "Codex slash/skill entry points",
		DetectPaths: []string{".codex", ".codex/skills", "AGENTS.md"},
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
				Description("Detected tools are pre-selected. File generation happens only after confirmation in the next setup step.").
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
