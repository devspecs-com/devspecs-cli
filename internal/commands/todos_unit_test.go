package commands

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterTodoRows(t *testing.T) {
	rows := []store.TodoRow{
		{Done: false, Text: "open"},
		{Done: true, Text: "done"},
	}
	open := filterTodoRows(rows, true, false)
	require.Len(t, open, 1, "open: %+v", open)
	assert.Equal(t, "open", open[0].Text, "open: %+v", open)

	done := filterTodoRows(rows, false, true)
	require.Len(t, done, 1, "done: %+v", done)
	assert.Equal(t, "done", done[0].Text, "done: %+v", done)

	all := filterTodoRows(rows, false, false)
	require.Len(t, all, 2,
		"all: %+v", all)

}
