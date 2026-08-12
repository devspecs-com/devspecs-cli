package commands

import (
	"fmt"
	"strings"
)

const (
	composeADRFormatAuto         = "auto"
	composeADRFormatNygard       = "nygard"
	composeADRFormatMADR         = "madr"
	composeADRFormatYStatement   = "y-statement"
	composeADRFormatOutcomeFirst = "outcome-first"
	composeADRFormatISO42010     = "iso-42010"
)

var composeADRFormats = []string{
	composeADRFormatAuto,
	composeADRFormatNygard,
	composeADRFormatMADR,
	composeADRFormatYStatement,
	composeADRFormatOutcomeFirst,
	composeADRFormatISO42010,
}

type composeTemplateInput struct {
	Title      string
	RecordID   string
	Variant    string
	Provenance string
}

func renderComposeADR(format string, input composeTemplateInput) (string, error) {
	var body string
	switch format {
	case composeADRFormatNygard:
		body = renderComposeNygard(input)
	case composeADRFormatMADR:
		body = renderComposeMADR(input)
	case composeADRFormatYStatement:
		body = renderComposeYStatement(input)
	case composeADRFormatOutcomeFirst:
		body = renderComposeOutcomeFirst(input)
	case composeADRFormatISO42010:
		body = renderComposeISO42010(input)
	default:
		return "", fmt.Errorf("unsupported ADR format %q", format)
	}
	return appendComposeProvenance(body, input.Provenance), nil
}

func renderComposeNygard(input composeTemplateInput) string {
	return fmt.Sprintf(`# ADR-%s: %s

## Status

Proposed

## Context

<Describe the problem, constraints, and why a decision is needed.>

## Decision

<State the settled technical choice.>

## Consequences

- Positive: <benefit>
- Negative: <cost or risk>
- Follow-up: <work or review required>
`, input.RecordID, input.Title)
}

func renderComposeMADR(input composeTemplateInput) string {
	if input.Variant == "minimal" {
		return fmt.Sprintf(`# %s

## Context and Problem Statement

<Describe the context, problem, constraints, and decision scope.>

## Considered Options

* <option one>
* <option two>

## Decision Outcome

Chosen option: "<option>", because <rationale>.

### Consequences

* Good, because <positive consequence>
* Bad, because <negative consequence>
`, input.Title)
	}
	return fmt.Sprintf(`---
status: "proposed"
date: "<YYYY-MM-DD>"
decision-makers: "<names or roles>"
consulted: "<names or roles>"
informed: "<names or roles>"
---

# %s

## Context and Problem Statement

<Describe the context, problem, constraints, and decision scope.>

## Decision Drivers

* <driver one>
* <driver two>

## Considered Options

* <option one>
* <option two>

## Decision Outcome

Chosen option: "<option>", because <rationale>.

### Consequences

* Good, because <positive consequence>
* Bad, because <negative consequence>

### Confirmation

<Describe how implementation or compliance will be confirmed.>

## Pros and Cons of the Options

### <option one>

* Good, because <argument>
* Bad, because <argument>

### <option two>

* Good, because <argument>
* Bad, because <argument>

## More Information

<Link evidence, related decisions, agreement, or revisit criteria.>
`, input.Title)
}

func renderComposeYStatement(input composeTemplateInput) string {
	return fmt.Sprintf(`# %s

## Y-Statement

In the context of **<context>**, facing **<concern>**, we have decided **for or against** *<subject>* in order to **<intended outcome>**, accepting that **<tradeoff>**.

## Fields

- **Context:** <context>
- **Concern:** <concern>
- **Stance / subject:** <for or against> / <subject>
- **Intended outcome:** <outcome>
- **Deliberate tradeoff:** <tradeoff>
`, input.Title)
}

func renderComposeOutcomeFirst(input composeTemplateInput) string {
	return fmt.Sprintf(`# ADR-%s: %s

## Status

Proposed

## Outcome

<What this decision is trying to make true.>

## Decision

<State the concrete technical choice.>

## Primary tradeoff

<State the main cost, risk, or limitation knowingly accepted.>

## Why

- <reason one>
- <reason two>

## Decision boundaries

Impacted:

- <systems, teams, interfaces, workflows, or constraints affected>

Not impacted:

- <systems, teams, interfaces, workflows, or constraints unchanged>

Assumptions:

- <assumption>

Guardrails:

- <ownership, rollback, SLO, test, or rollout constraint>
`, input.RecordID, input.Title)
}

func renderComposeISO42010(input composeTemplateInput) string {
	return fmt.Sprintf(`# %s

> **ISO 42010 Companion.** Use this shape when a decision must connect stakeholders, concerns, views, viewpoints, and traceability. It is not a conformity template.

**Record ID / cross-ref:** ADR-42010-%s

## System / subject scope

<Describe the system of interest and scope.>

## Stakeholders and roles

<Name stakeholders and their roles.>

## Stakeholder concerns

<List the concerns the architecture description must address.>

## View and viewpoint

<Describe what is shown and from which perspective.>

## Decision

<State the technical choice.>

## Rationale

<Explain the choice with respect to concerns and alternatives.>

## Architectural impact

<Describe affected elements, relationships, and operational follow-through.>

## Traceability

<Link requirements, policy, SLOs, views, and related ADRs.>
`, input.Title, input.RecordID)
}

func renderComposeRFC(input composeTemplateInput) string {
	body := fmt.Sprintf(`# RFC: %s

## Status

Draft

## Summary

<Summarize the proposed direction.>

## Motivation

<Describe the problem, evidence, and why review is needed now.>

## Goals

- <goal>

## Non-goals

- <explicitly excluded outcome>

## Proposal

<Describe the proposed design, interfaces, and boundaries.>

## Alternatives

- <alternative and why it was not selected for this proposal>

## Risks and rollout

<Describe risks, migration, validation, and rollback.>

## Open questions

- <question requiring review>
`, input.Title)
	return appendComposeProvenance(body, input.Provenance)
}

func renderComposePRD(input composeTemplateInput) string {
	body := fmt.Sprintf(`# PRD: %s

## Status

Draft

## Problem

<Describe the user or product problem and supporting evidence.>

## Users

- <primary user or stakeholder>

## Desired outcomes

- <observable outcome>

## Requirements

- <required behavior or constraint>

## Non-goals

- <explicitly excluded scope>

## Success measures

- <metric or acceptance signal>

## Risks and open questions

- <risk or unresolved question>
`, input.Title)
	return appendComposeProvenance(body, input.Provenance)
}

func appendComposeProvenance(body, provenance string) string {
	body = strings.TrimRight(body, "\r\n")
	provenance = strings.TrimSpace(provenance)
	if provenance == "" {
		return body + "\n"
	}
	return body + "\n\n" + provenance + "\n"
}

func detectComposeADRFormat(body string) (string, string) {
	lower := strings.ToLower(strings.ReplaceAll(body, "\r\n", "\n"))
	switch {
	case strings.Contains(lower, "iso 42010 companion") ||
		(strings.Contains(lower, "## system / subject scope") && strings.Contains(lower, "## view and viewpoint")):
		return composeADRFormatISO42010, ""
	case strings.Contains(lower, "## primary tradeoff") && strings.Contains(lower, "## decision boundaries"):
		return composeADRFormatOutcomeFirst, ""
	case strings.Contains(lower, "## y-statement") || strings.Contains(lower, "# y-statement"):
		return composeADRFormatYStatement, ""
	case strings.Contains(lower, "## context and problem statement") && strings.Contains(lower, "## decision outcome"):
		if strings.Contains(lower, "## decision drivers") || strings.Contains(lower, "## pros and cons of the options") {
			return composeADRFormatMADR, "full"
		}
		return composeADRFormatMADR, "minimal"
	case strings.Contains(lower, "## context") && strings.Contains(lower, "## decision") && strings.Contains(lower, "## consequences"):
		return composeADRFormatNygard, ""
	default:
		return "", ""
	}
}
