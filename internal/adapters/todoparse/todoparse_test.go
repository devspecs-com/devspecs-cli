package todoparse_test

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
)

func TestParse_TasksOnlyTodos(t *testing.T) {
	md := "## Tasks\n\n- [ ] One\n- [x] Two\n"
	pr := todoparse.Parse(md, "p.md")
	if len(pr.Todos) != 2 || len(pr.Criteria) != 0 {
		t.Fatalf("want 2 todos 0 criteria, got todos=%d criteria=%d", len(pr.Todos), len(pr.Criteria))
	}
	if pr.Todos[0].Text != "One" || pr.Todos[0].Done {
		t.Errorf("todo0: %+v", pr.Todos[0])
	}
	if !pr.Todos[1].Done {
		t.Errorf("todo1 should be done: %+v", pr.Todos[1])
	}
}

func TestParse_AuditableSuccessAsCriteria(t *testing.T) {
	md := "## Tasks\n\n- [ ] Task A\n\n## Auditable success criteria\n\n- [ ] Must pass integration\n"
	pr := todoparse.Parse(md, "plan.md")
	if len(pr.Todos) != 1 || len(pr.Criteria) != 1 {
		t.Fatalf("want 1 todo 1 criterion, got todos=%d criteria=%d", len(pr.Todos), len(pr.Criteria))
	}
	if pr.Todos[0].Text != "Task A" {
		t.Errorf("todo text: %q", pr.Todos[0].Text)
	}
	if pr.Criteria[0].CriteriaKind != todoparse.KindSuccess {
		t.Errorf("criteria kind: want %q got %q", todoparse.KindSuccess, pr.Criteria[0].CriteriaKind)
	}
	if pr.Criteria[0].Text != "Must pass integration" {
		t.Errorf("criterion text: %q", pr.Criteria[0].Text)
	}
}

func TestParse_OKRHeading(t *testing.T) {
	md := "### OKRs\n\n- [ ] Ship v1\n"
	pr := todoparse.Parse(md, "x.md")
	if len(pr.Todos) != 0 || len(pr.Criteria) != 1 {
		t.Fatalf("want 0 todos 1 criterion, got todos=%d criteria=%d", len(pr.Todos), len(pr.Criteria))
	}
	if pr.Criteria[0].CriteriaKind != todoparse.KindOKR {
		t.Errorf("kind: %q", pr.Criteria[0].CriteriaKind)
	}
}

func TestParse_AcceptanceHeading(t *testing.T) {
	md := "## Acceptance criteria\n\n- [ ] AC1\n"
	pr := todoparse.Parse(md, "x.md")
	if len(pr.Criteria) != 1 || pr.Criteria[0].CriteriaKind != todoparse.KindAcceptance {
		t.Fatalf("got %+v", pr.Criteria)
	}
}

func TestParse_NoHeadingDefaultsToTodo(t *testing.T) {
	md := "- [ ] orphan\n"
	pr := todoparse.Parse(md, "x.md")
	if len(pr.Todos) != 1 || len(pr.Criteria) != 0 {
		t.Fatalf("got todos=%d criteria=%d", len(pr.Todos), len(pr.Criteria))
	}
}

func TestParse_IgnoresChecklistsInFencedBlock(t *testing.T) {
	md := "## Success criteria\n\n```\n- [ ] fake\n```\n\n- [ ] real\n"
	pr := todoparse.Parse(md, "x.md")
	if len(pr.Criteria) != 1 || pr.Criteria[0].Text != "real" {
		t.Fatalf("got %+v", pr.Criteria)
	}
}
