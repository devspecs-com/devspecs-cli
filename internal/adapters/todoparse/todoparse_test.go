package todoparse_test

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_TasksOnlyTodos(t *testing.T) {
	md := "## Tasks\n\n- [ ] One\n- [x] Two\n"

	pr := todoparse.Parse(md, "p.md")

	require.Len(t, pr.Todos, 2)
	assert.Empty(t, pr.Criteria)
	assert.Equal(t, "One", pr.Todos[0].Text)
	assert.False(t, pr.Todos[0].Done)
	assert.Equal(t, "Two", pr.Todos[1].Text)
	assert.True(t, pr.Todos[1].Done)
}

func TestParse_AuditableSuccessAsCriteria(t *testing.T) {
	md := "## Tasks\n\n- [ ] Task A\n\n## Auditable success criteria\n\n- [ ] Must pass integration\n"

	pr := todoparse.Parse(md, "plan.md")

	require.Len(t, pr.Todos, 1)
	require.Len(t, pr.Criteria, 1)
	assert.Equal(t, "Task A", pr.Todos[0].Text)
	assert.Equal(t, todoparse.KindSuccess, pr.Criteria[0].CriteriaKind)
	assert.Equal(t, "Must pass integration", pr.Criteria[0].Text)
}

func TestParse_OKRHeading(t *testing.T) {
	md := "### OKRs\n\n- [ ] Ship v1\n"

	pr := todoparse.Parse(md, "x.md")

	assert.Empty(t, pr.Todos)
	require.Len(t, pr.Criteria, 1)
	assert.Equal(t, todoparse.KindOKR, pr.Criteria[0].CriteriaKind)
}

func TestParse_AcceptanceHeading(t *testing.T) {
	md := "## Acceptance criteria\n\n- [ ] AC1\n"

	pr := todoparse.Parse(md, "x.md")

	require.Len(t, pr.Criteria, 1)
	assert.Equal(t, todoparse.KindAcceptance, pr.Criteria[0].CriteriaKind)
}

func TestParse_NoHeadingDefaultsToTodo(t *testing.T) {
	md := "- [ ] orphan\n"

	pr := todoparse.Parse(md, "x.md")

	require.Len(t, pr.Todos, 1)
	assert.Empty(t, pr.Criteria)
}

func TestParse_IgnoresChecklistsInFencedBlock(t *testing.T) {
	md := "## Success criteria\n\n```\n- [ ] fake\n```\n\n- [ ] real\n"

	pr := todoparse.Parse(md, "x.md")

	require.Len(t, pr.Criteria, 1)
	assert.Equal(t, "real", pr.Criteria[0].Text)
}
