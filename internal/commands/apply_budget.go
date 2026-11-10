package commands

import (
	"crypto/sha256"
	"fmt"
	"unicode/utf8"
)

type applyInputBudget struct {
	SchemaVersion       int                   `json:"schema_version"`
	PromptBytes         int                   `json:"prompt_bytes"`
	PromptTokenEstimate int                   `json:"prompt_token_estimate"`
	TokenEstimateMethod string                `json:"token_estimate_method"`
	ContextArtifacts    []applyBudgetArtifact `json:"context_artifacts"`
	Omissions           []string              `json:"omissions"`
}

type applyBudgetArtifact struct {
	JSONPointer string `json:"json_pointer"`
	SizeBytes   int    `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	InPrompt    bool   `json:"in_prompt"`
}

func measureApplyInput(prompt string, target taskTargetOutput) applyInputBudget {
	prompt = applyJSONText(prompt)
	budget := applyInputBudget{
		SchemaVersion:       1,
		PromptBytes:         len(prompt),
		PromptTokenEstimate: approximateTokenCount(prompt),
		TokenEstimateMethod: "ceil_utf8_bytes_div_4",
		ContextArtifacts:    []applyBudgetArtifact{},
		Omissions:           []string{"referenced_files_not_measured"},
	}
	if target.PlanBody == "" {
		budget.Omissions = append(budget.Omissions, "plan_body_empty_or_unavailable")
		return budget
	}
	plan := applyJSONText(target.PlanBody)
	budget.ContextArtifacts = append(budget.ContextArtifacts, applyBudgetArtifact{
		JSONPointer: "/target_context/plan_body",
		SizeBytes:   len(plan),
		SHA256:      fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(plan))),
		InPrompt:    target.Kind != "closeout",
	})
	return budget
}

func applyJSONText(text string) string {
	// Match encoding/json's replacement of each invalid UTF-8 byte.
	if !utf8.ValidString(text) {
		return string([]rune(text))
	}
	return text
}
