package sourcecontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellImportResolver_DynamicLibraryReference_ResolvesUniqueSuffix(t *testing.T) {
	resolver := NewShellImportResolver([]string{"bin/tool", "lib/core.sh"})

	got := resolver.Resolve("bin/tool", "$TOOL_LIB/core.sh")

	assert.Equal(t, "lib/core.sh", got)
}

func TestShellImportResolver_MultipleDynamicDirectories_ResolvesUniqueStaticSuffix(t *testing.T) {
	resolver := NewShellImportResolver([]string{
		"lib/bats-core/formatter.bash",
		"libexec/bats-core/bats-format-tap",
	})

	got := resolver.Resolve("libexec/bats-core/bats-format-tap", "$BATS_ROOT/$BATS_LIBDIR/bats-core/formatter.bash")

	assert.Equal(t, "lib/bats-core/formatter.bash", got)
}

func TestShellImportResolver_ParentRelativeReference_ResolvesRootSource(t *testing.T) {
	resolver := NewShellImportResolver([]string{"nvm.sh", "test/fast/Unit tests/nvm_validate_install"})

	got := resolver.Resolve("test/fast/Unit tests/nvm_validate_install", "../../../nvm.sh")

	assert.Equal(t, "nvm.sh", got)
}

func TestShellImportResolver_VariableDirectoryReference_ResolvesBatsHelperExtension(t *testing.T) {
	resolver := NewShellImportResolver([]string{"test/formatter.bats", "test/test_helper.bash"})

	got := resolver.Resolve("test/formatter.bats", "${BATS_TEST_DIRNAME}/test_helper")

	assert.Equal(t, "test/test_helper.bash", got)
}

func TestShellImportResolver_AmbiguousBasename_ReturnsEmpty(t *testing.T) {
	resolver := NewShellImportResolver([]string{"lib/core.sh", "fixtures/core.sh", "bin/tool"})

	got := resolver.Resolve("bin/tool", "$TOOL_LIB/core.sh")

	assert.Empty(t, got)
}
