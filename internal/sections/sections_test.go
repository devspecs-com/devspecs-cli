package sections

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractMarkdown_WithFrontmatterAndNestedHeadings_ReturnsSectionEvidence(t *testing.T) {
	body := "---\r\nstatus: active\r\nowner = \"platform\"\r\n# comment\r\n---\r\n# Plan\r\nIntro [docs](docs/plan.md)\r\n## Acceptance Criteria\r\n- request succeeds\r\n- [ ] add retry\r\n- [x] shipped\r\n[docs](docs/plan.md)\r\n```go\r\n# Not A Heading\r\n```\r\n### Detail ###\r\nSee [runbook](ops/runbook.md)."

	sections := ExtractMarkdown(body)

	require.Len(t, sections, 3)
	assert.Equal(t, "Plan", sections[0].Title)
	assert.Equal(t, "Plan", sections[0].HeadingPath)
	assert.Equal(t, 1, sections[0].HeadingDepth)
	assert.Equal(t, 6, sections[0].StartLine)
	assert.Equal(t, 7, sections[0].EndLine)
	assert.Equal(t, "Intro [docs](docs/plan.md)", sections[0].Body)
	require.Len(t, sections[0].Frontmatter, 2)
	assert.Equal(t, "active", sections[0].Frontmatter["status"])
	assert.Equal(t, "platform", sections[0].Frontmatter["owner"])
	require.Len(t, sections[0].Links, 1)
	assert.Equal(t, "docs/plan.md", sections[0].Links[0])

	assert.Equal(t, "Acceptance Criteria", sections[1].Title)
	assert.Equal(t, "Plan > Acceptance Criteria", sections[1].HeadingPath)
	assert.Equal(t, 2, sections[1].HeadingDepth)
	assert.Equal(t, 8, sections[1].StartLine)
	assert.Equal(t, 15, sections[1].EndLine)
	require.Len(t, sections[1].Tasks, 2)
	assert.Equal(t, "- [ ] add retry", sections[1].Tasks[0])
	assert.Equal(t, "- [x] shipped", sections[1].Tasks[1])
	require.Len(t, sections[1].AcceptanceCriteria, 3)
	assert.Equal(t, "- request succeeds", sections[1].AcceptanceCriteria[0])
	assert.Equal(t, "- [ ] add retry", sections[1].AcceptanceCriteria[1])
	assert.Equal(t, "- [x] shipped", sections[1].AcceptanceCriteria[2])
	require.Len(t, sections[1].Links, 1)
	assert.Equal(t, "docs/plan.md", sections[1].Links[0])

	assert.Equal(t, "Detail", sections[2].Title)
	assert.Equal(t, "Plan > Acceptance Criteria > Detail", sections[2].HeadingPath)
	assert.Equal(t, 3, sections[2].HeadingDepth)
	assert.Equal(t, 16, sections[2].StartLine)
	assert.Equal(t, 17, sections[2].EndLine)
	require.Len(t, sections[2].Links, 1)
	assert.Equal(t, "ops/runbook.md", sections[2].Links[0])
}

func TestExtractMarkdown_WithoutHeadings_ReturnsEmptySections(t *testing.T) {
	body := "plain text\n- [ ] task without a heading"

	sections := ExtractMarkdown(body)

	assert.Empty(t, sections)
}

func TestExtractMarkdown_WithUnclosedFrontmatter_StillFindsHeadings(t *testing.T) {
	body := "---\nstatus: draft\n# Recovery\nBody"

	sections := ExtractMarkdown(body)

	require.Len(t, sections, 1)
	assert.Equal(t, "Recovery", sections[0].Title)
	assert.Nil(t, sections[0].Frontmatter)
}

func TestExtractMarkdown_WithHeadingOnly_SetsEmptyBodyAndHeadingEndLine(t *testing.T) {
	sections := ExtractMarkdown("# Empty")

	require.Len(t, sections, 1)
	assert.Equal(t, 1, sections[0].StartLine)
	assert.Equal(t, 1, sections[0].EndLine)
	assert.Empty(t, sections[0].Body)
	assert.Empty(t, sections[0].Tasks)
	assert.Empty(t, sections[0].Links)
}

func TestAssignStableIDs_AddsParentAndSourceMetadata(t *testing.T) {
	sections := ExtractMarkdown("# Plan\nBody\n## Detail\nMore")

	assigned := AssignStableIDs(sections, "artifact-1", "revision-2", "docs/plan.md")

	require.Len(t, assigned, 2)
	assert.NotEmpty(t, assigned[0].ID)
	assert.NotEmpty(t, assigned[1].ID)
	assert.NotEqual(t, assigned[0].ID, assigned[1].ID)
	assert.Equal(t, "artifact-1", assigned[0].ArtifactID)
	assert.Equal(t, "revision-2", assigned[0].RevisionID)
	assert.Equal(t, "docs/plan.md", assigned[0].SourcePath)
	assert.Equal(t, "artifact-1", assigned[1].ArtifactID)
	assert.Equal(t, "revision-2", assigned[1].RevisionID)
	assert.Equal(t, "docs/plan.md", assigned[1].SourcePath)
}

func TestStableID_WithSameEvidence_ReturnsDeterministicID(t *testing.T) {
	first := StableID("artifact-1", "revision-2", "Plan > Detail", 4, 8)
	second := StableID("artifact-1", "revision-2", "Plan > Detail", 4, 8)

	assert.Equal(t, first, second)
	assert.Regexp(t, `^sec_[0-9a-f]{16}$`, first)
}

func TestStableID_WithDifferentLineRange_ReturnsDifferentID(t *testing.T) {
	first := StableID("artifact-1", "revision-2", "Plan", 1, 3)
	second := StableID("artifact-1", "revision-2", "Plan", 1, 4)

	assert.NotEqual(t, first, second)
}

func TestEnclosingSectionID_WithContainedLine_ReturnsSectionID(t *testing.T) {
	sections := []Section{{ID: "sec_one", StartLine: 3, EndLine: 7}}

	id := EnclosingSectionID(sections, 7)

	assert.Equal(t, "sec_one", id)
}

func TestEnclosingSectionID_WithNonPositiveLine_ReturnsEmpty(t *testing.T) {
	id := EnclosingSectionID([]Section{{ID: "sec_one", StartLine: 1, EndLine: 2}}, 0)

	assert.Empty(t, id)
}

func TestEnclosingSectionID_WithSectionMissingID_ReturnsEmpty(t *testing.T) {
	id := EnclosingSectionID([]Section{{StartLine: 1, EndLine: 2}}, 1)

	assert.Empty(t, id)
}

func TestApproxTokenCount_WithEmptyText_ReturnsZero(t *testing.T) {
	count := ApproxTokenCount("")

	assert.Zero(t, count)
}

func TestApproxTokenCount_WithPartialToken_RoundsUp(t *testing.T) {
	count := ApproxTokenCount("12345")

	assert.Equal(t, 2, count)
}
