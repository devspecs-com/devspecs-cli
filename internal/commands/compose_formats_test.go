package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderComposeADR_WithNygard_RendersCompactDecisionSections(t *testing.T) {
	input := composeTemplateInput{Title: "Use PostgreSQL", RecordID: "0007"}

	actual, err := renderComposeADR(composeADRFormatNygard, input)

	require.NoError(t, err)
	assert.Contains(t, actual, "# ADR-0007: Use PostgreSQL")
	assert.Contains(t, actual, "## Context")
	assert.Contains(t, actual, "## Decision")
	assert.Contains(t, actual, "## Consequences")
}

func TestRenderComposeADR_WithMADRFull_RendersComparisonAndConfirmationSections(t *testing.T) {
	input := composeTemplateInput{Title: "Use PostgreSQL", RecordID: "0007", Variant: "full"}

	actual, err := renderComposeADR(composeADRFormatMADR, input)

	require.NoError(t, err)
	assert.Contains(t, actual, "status: \"proposed\"")
	assert.Contains(t, actual, "## Context and Problem Statement")
	assert.Contains(t, actual, "## Decision Drivers")
	assert.Contains(t, actual, "## Pros and Cons of the Options")
	assert.Contains(t, actual, "### Confirmation")
}

func TestRenderComposeADR_WithMADRMinimal_OmitsFullComparisonSections(t *testing.T) {
	input := composeTemplateInput{Title: "Use PostgreSQL", RecordID: "0007", Variant: "minimal"}

	actual, err := renderComposeADR(composeADRFormatMADR, input)

	require.NoError(t, err)
	assert.Contains(t, actual, "## Context and Problem Statement")
	assert.Contains(t, actual, "## Considered Options")
	assert.Contains(t, actual, "## Decision Outcome")
	assert.NotContains(t, actual, "## Decision Drivers")
	assert.NotContains(t, actual, "## Pros and Cons of the Options")
}

func TestRenderComposeADR_WithYStatement_RendersSentenceAndReviewFields(t *testing.T) {
	input := composeTemplateInput{Title: "Use PostgreSQL", RecordID: "0007"}

	actual, err := renderComposeADR(composeADRFormatYStatement, input)

	require.NoError(t, err)
	assert.Contains(t, actual, "## Y-Statement")
	assert.Contains(t, actual, "In the context of **<context>**")
	assert.Contains(t, actual, "## Fields")
}

func TestRenderComposeADR_WithOutcomeFirst_RendersOutcomeAndDecisionBoundaries(t *testing.T) {
	input := composeTemplateInput{Title: "Use PostgreSQL", RecordID: "0007"}

	actual, err := renderComposeADR(composeADRFormatOutcomeFirst, input)

	require.NoError(t, err)
	assert.Contains(t, actual, "# ADR-0007: Use PostgreSQL")
	assert.Contains(t, actual, "## Outcome")
	assert.Contains(t, actual, "## Primary tradeoff")
	assert.Contains(t, actual, "## Decision boundaries")
}

func TestRenderComposeADR_WithISO42010_RendersArchitectureDescriptionTraceability(t *testing.T) {
	input := composeTemplateInput{Title: "Use PostgreSQL", RecordID: "0007"}

	actual, err := renderComposeADR(composeADRFormatISO42010, input)

	require.NoError(t, err)
	assert.Contains(t, actual, "**ISO 42010 Companion.**")
	assert.Contains(t, actual, "## Stakeholders and roles")
	assert.Contains(t, actual, "## View and viewpoint")
	assert.Contains(t, actual, "## Traceability")
}

func TestRenderComposeADR_WithUnsupportedFormat_ReturnsError(t *testing.T) {
	input := composeTemplateInput{Title: "Use PostgreSQL", RecordID: "0007"}

	actual, err := renderComposeADR("unsupported", input)

	require.Error(t, err)
	assert.Empty(t, actual)
	assert.Contains(t, err.Error(), "unsupported ADR format")
}

func TestDetectComposeADRFormat_WithOutcomeFirst_ReturnsOutcomeFirst(t *testing.T) {
	body := "# Decision\n\n## Outcome\n\nResult\n\n## Decision\n\nChoice\n\n## Primary tradeoff\n\nCost\n\n## Decision boundaries\n\nScope\n"

	format, variant := detectComposeADRFormat(body)

	assert.Equal(t, composeADRFormatOutcomeFirst, format)
	assert.Empty(t, variant)
}

func TestDetectComposeADRFormat_WithMADRMinimal_ReturnsMinimalVariant(t *testing.T) {
	body := "# Decision\n\n## Context and Problem Statement\n\nProblem\n\n## Considered Options\n\nOptions\n\n## Decision Outcome\n\nChoice\n"

	format, variant := detectComposeADRFormat(body)

	assert.Equal(t, composeADRFormatMADR, format)
	assert.Equal(t, "minimal", variant)
}

func TestDetectComposeADRFormat_WithNygard_ReturnsNygard(t *testing.T) {
	body := "# Decision\n\n## Context\n\nProblem\n\n## Decision\n\nChoice\n\n## Consequences\n\nCost\n"

	format, variant := detectComposeADRFormat(body)

	assert.Equal(t, composeADRFormatNygard, format)
	assert.Empty(t, variant)
}

func TestDetectComposeADRFormat_WithYStatement_ReturnsYStatement(t *testing.T) {
	body := "# Decision\n\n## Y-Statement\n\nIn the context of a local index, we decided to serialize writers.\n"

	format, variant := detectComposeADRFormat(body)

	assert.Equal(t, composeADRFormatYStatement, format)
	assert.Empty(t, variant)
}

func TestDetectComposeADRFormat_WithISO42010Companion_ReturnsISO42010(t *testing.T) {
	body := "# Decision\n\n> ISO 42010 Companion\n\n## Stakeholder concerns\n\nReliability\n"

	format, variant := detectComposeADRFormat(body)

	assert.Equal(t, composeADRFormatISO42010, format)
	assert.Empty(t, variant)
}
