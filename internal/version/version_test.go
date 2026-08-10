package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaults(t *testing.T) {
	assert.Equal(t, "dev", Version)
	assert.Equal(t, "none", Commit)
	assert.Equal(t, "unknown", Date)
}
