package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion_Default_IsDev(t *testing.T) {
	actual := Version

	assert.Equal(t, "dev", actual)
}

func TestCommit_Default_IsNone(t *testing.T) {
	actual := Commit

	assert.Equal(t, "none", actual)
}

func TestDate_Default_IsUnknown(t *testing.T) {
	actual := Date

	assert.Equal(t, "unknown", actual)
}
