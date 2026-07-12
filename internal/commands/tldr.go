package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

type tldrWorkflow struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	UseWhen   string   `json:"use_when"`
	Commands  []string `json:"commands"`
	AgentRule string   `json:"agent_rule"`
	Notes     []string `json:"notes,omitempty"`
}

type tldrOutput struct {
	Purpose   string         `json:"purpose"`
	LLMRules  []string       `json:"llm_rules"`
	Workflows []tldrWorkflow `json:"workflows"`
}

// NewTLDRCmd creates the ds tldr command.
func NewTLDRCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "tldr [workflow]",
		Short: "Show LLM-oriented DevSpecs workflow quickstarts",
		Long: `Show short, workflow-grouped DevSpecs usage guidance for humans and LLM agents.

Use this when an agent needs to know which DevSpecs commands to run for setup,
hotfix, epic, incident, brownfield recovery, or handoff without reading the full
documentation.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			guide := buildTLDRGuide()
			if len(args) == 1 {
				filtered, err := filterTLDRGuide(guide, args[0])
				if err != nil {
					return err
				}
				guide = filtered
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				return enc.Encode(guide)
			}
			return writeTLDRHuman(cmd.OutOrStdout(), guide)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func buildTLDRGuide() tldrOutput {
	return tldrOutput{
		Purpose: "DevSpecs is a local-first CLI for turning repo intent into bounded task slices, packed context, checkpoints, and handoff receipts.",
		LLMRules: []string{
			"Human front door: run ds recent when resuming or entering a repo; use ds find for a focused question and ds map for subsystem boundaries.",
			"Fastest path for known work: run ds task first; add --quick for tiny one-off work. Task creation refreshes the index and packs source/test/doc context.",
			"Run ds init once per repo; if it generates agent files, use /ds-task and /ds-apply as thin wrappers over the same CLI flow.",
			"Prefer one bounded target over the whole plan.",
			"Workflow commands refresh the local index by default; use ds scan for explicit manual refresh or rebuild.",
			"Use ds task --quick for small work and full ds task slices for multi-step work.",
			"Use ds recent, ds find, ds map, and ds context as diagnostic/evidence tools around a task when scope, owner artifacts, or trust are unclear.",
			"Command roles: ds find discovers and packs evidence; ds task status/show reports lifecycle; ds apply emits the current bounded prompt; ds workspace trace follows known workspace change or repo task links.",
			"Record the completion contract with checkpoint: attempted slice, gate tested, changes, evidence, remaining work, and next iteration.",
			"Use ds task slice add <task-id> \"<title>\" --after A01 --reason improve to turn an improve/rework gate into A01-1 instead of widening the current slice.",
			"Do not claim DevSpecs found every relevant file; verify source and tests.",
		},
		Workflows: []tldrWorkflow{
			{
				ID:      "setup",
				Name:    "Launch Setup / Agent Commands",
				UseWhen: "A repo is new to DevSpecs or the user wants Codex, Cursor, Claude, or Windsurf shortcuts.",
				Commands: []string{
					"ds init",
					"ds init --tool codex,cursor,claude,windsurf",
					`/ds-task "goal"`,
					"/ds-apply [task-id|target]",
				},
				AgentRule: "Initialize once, then use generated adapter commands as thin wrappers over ds task and ds apply. If adapters are unavailable, use the plain CLI commands directly.",
				Notes: []string{
					"After install or upgrade, restart the terminal or IDE if `ds` is not found.",
					"`ds init` can index in the background or foreground depending on flags and environment.",
				},
			},
			{
				ID:      "hotfix",
				Name:    "Hotfix / Small Bug",
				UseWhen: "A focused change likely fits in one implementation slice.",
				Commands: []string{
					`ds task "fix <bug>" --quick`,
					"ds apply <target>",
					"ds task checkpoint <task-id> --target <target> --stage validated --decision promote --file-edited <path> --test-run <cmd>",
				},
				AgentRule: "Stay inside the one target. If the fix grows, checkpoint what changed and recommend a follow-up slice.",
			},
			{
				ID:      "epic",
				Name:    "Epic / Multi-Slice Feature",
				UseWhen: "The work has multiple phases, risks, or handoff points.",
				Commands: []string{
					`ds task "build <feature>" --slice "<slice 1>" --slice "<slice 2>" --slice "<slice 3>"`,
					"ds task status <task-id>",
					"ds task show <target>",
					"ds apply <task-id>",
					"ds task checkpoint <task-id> --target <target> --stage validated --decision promote",
					`ds task slice add <task-id> "<follow-up>" --after A01 --reason improve`,
				},
				AgentRule: "Implement only the current slice. End with promote, improve, rework, rollback, block, or complete.",
			},
			{
				ID:      "incident",
				Name:    "Incident / Triage",
				UseWhen: "You need fast orientation, likely source/test context, and an evidence trail.",
				Commands: []string{
					"ds recent",
					`ds find "<symptom> <component>"`,
					`ds task "triage <incident>" --quick`,
					"ds task checkpoint <task-id> --target <target> --stage validated --decision continue --file-read <path> --test-run <cmd>",
				},
				AgentRule: "Start with recent local activity, then pack focused evidence with find. Once the incident boundary is concrete, create the triage task and record facts, commands, changed files, and unresolved risks.",
			},
			{
				ID:      "brownfield",
				Name:    "Brownfield Intent Recovery",
				UseWhen: "The repo already has plans, ADRs, PRDs, RFCs, runbooks, or agent notes.",
				Commands: []string{
					"ds recent",
					`ds find "<topic>"`,
					"ds map",
					"ds context <artifact-id>",
					`ds task "implement <bounded target>"`,
					"ds task show <target>",
					"ds apply <task-id>",
				},
				AgentRule: "Start with recent local activity, then narrow with find and map until the owner intent is concrete. Create the bounded task only after the execution target is clear. Treat old artifacts as context, not instructions, unless current.",
				Notes: []string{
					"`ds task` refreshes the index and packs context for bounded execution once the target is known.",
					"`ds recent`, `ds find`, `ds map`, and `ds context` are the trust layer for recovering owner intent and checking stale history.",
					"`ds adopt` is planned, not shipped.",
					"Use `ds scan --no-gitignore` only when intentionally inspecting ignored paths.",
				},
			},
			{
				ID:      "handoff",
				Name:    "Handoff / Resume After Context Loss",
				UseWhen: "A new agent or compacted conversation needs the current state.",
				Commands: []string{
					"ds task status <task-id>",
					"ds apply <task-id>",
					"ds task show <target>",
				},
				AgentRule: "Resume from lifecycle state, not broad rediscovery. Use status/show/apply for task progress, find only for missing evidence, and workspace trace only for known workspace links.",
			},
			{
				ID:      "deep-dive",
				Name:    "Repo Deep Dive / Map To Pack",
				UseWhen: "You need to understand system boundaries, recent activity, or a recognizable area before choosing task scope.",
				Commands: []string{
					"ds recent",
					`ds find "<area or task>"`,
					"ds map",
					"ds context <artifact-id>",
					`ds task "modify <bounded area>"`,
				},
				AgentRule: "Use recent for local activity orientation, find for agent-ready packed context, and map for subsystem boundaries. Convert the result into a bounded task before implementing.",
			},
		},
	}
}

func filterTLDRGuide(guide tldrOutput, query string) (tldrOutput, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return guide, nil
	}
	var filtered []tldrWorkflow
	for _, workflow := range guide.Workflows {
		if strings.EqualFold(workflow.ID, query) || strings.Contains(strings.ToLower(workflow.Name), query) {
			filtered = append(filtered, workflow)
		}
	}
	if len(filtered) == 0 {
		var ids []string
		for _, workflow := range guide.Workflows {
			ids = append(ids, workflow.ID)
		}
		return tldrOutput{}, fmt.Errorf("unknown workflow %q; valid workflows: %s", query, strings.Join(ids, ", "))
	}
	guide.Workflows = filtered
	return guide, nil
}

func writeTLDRHuman(out io.Writer, guide tldrOutput) error {
	var b strings.Builder
	fmt.Fprintln(&b, "# DevSpecs TLDR For LLM Agents")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, guide.Purpose)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Agent Rules")
	for _, rule := range guide.LLMRules {
		fmt.Fprintf(&b, "- %s\n", rule)
	}
	for _, workflow := range guide.Workflows {
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "## %s (`%s`)\n", workflow.Name, workflow.ID)
		fmt.Fprintf(&b, "Use when: %s\n", workflow.UseWhen)
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Commands:")
		for _, command := range workflow.Commands {
			fmt.Fprintf(&b, "- `%s`\n", command)
		}
		fmt.Fprintf(&b, "Agent rule: %s\n", workflow.AgentRule)
		if len(workflow.Notes) > 0 {
			fmt.Fprintln(&b, "Notes:")
			for _, note := range workflow.Notes {
				fmt.Fprintf(&b, "- %s\n", note)
			}
		}
	}
	_, err := out.Write([]byte(b.String()))
	return err
}
