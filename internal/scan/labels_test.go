package scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceTypeDisplayLabel_WithMarkdown_ReturnsPlanningDocs(t *testing.T) {
	actual := SourceTypeDisplayLabel("markdown")

	assert.Equal(t, "Planning docs", actual)
}

func TestSourceTypeDisplayLabel_WithOpenSpec_ReturnsOpenSpec(t *testing.T) {
	actual := SourceTypeDisplayLabel("openspec")

	assert.Equal(t, "OpenSpec", actual)
}

func TestSourceTypeDisplayLabel_WithADR_ReturnsADRs(t *testing.T) {
	actual := SourceTypeDisplayLabel("adr")

	assert.Equal(t, "ADRs", actual)
}

func TestSourceTypeDisplayLabel_WithSourceContext_ReturnsSourceContext(t *testing.T) {
	actual := SourceTypeDisplayLabel("source_context")

	assert.Equal(t, "Source context", actual)
}

func TestSourceTypeDisplayLabel_WithTestCase_ReturnsTestCases(t *testing.T) {
	actual := SourceTypeDisplayLabel("test_case")

	assert.Equal(t, "Test cases", actual)
}

func TestSourceTypeDisplayLabel_WithCodeComment_ReturnsCodeComments(t *testing.T) {
	actual := SourceTypeDisplayLabel("code_comment")

	assert.Equal(t, "Code comments", actual)
}

func TestSourceTypeDisplayLabel_WithUnknownType_ReturnsInput(t *testing.T) {
	actual := SourceTypeDisplayLabel("capture")

	assert.Equal(t, "capture", actual)
}
