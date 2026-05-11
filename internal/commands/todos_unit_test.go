package commands

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestFilterTodoRows(t *testing.T) {
	rows := []store.TodoRow{
		{Done: false, Text: "open"},
		{Done: true, Text: "done"},
	}
	open := filterTodoRows(rows, true, false)
	if len(open) != 1 || open[0].Text != "open" {
		t.Fatalf("open: %+v", open)
	}
	done := filterTodoRows(rows, false, true)
	if len(done) != 1 || done[0].Text != "done" {
		t.Fatalf("done: %+v", done)
	}
	all := filterTodoRows(rows, false, false)
	if len(all) != 2 {
		t.Fatalf("all: %+v", all)
	}
}
