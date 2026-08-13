package sourcecontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractShellSemantics_WithEntrypoint_ExtractsStaticFunctionsImportsAndDispatch(t *testing.T) {
	body := `#!/usr/bin/env bash
source "$ROOT/lib/core.sh"
dynamic="cmd_${verb}"
cmd_spawn() {
  echo spawn
}
main() {
  case "$1" in
    spawn) cmd_spawn "$@" ;;
    *) "$dynamic" ;;
  esac
}
`

	semantics, err := ExtractShellSemantics("bin/tool", "implementation", body)

	require.NoError(t, err)
	require.Len(t, semantics.Symbols, 3)
	assert.Equal(t, "spawn", semantics.Symbols[0].Name)
	assert.Equal(t, "dispatch", semantics.Symbols[0].Kind)
	assert.Equal(t, "cmd_spawn", semantics.Symbols[0].Parent)
	assert.Equal(t, 9, semantics.Symbols[0].Line)
	assert.Equal(t, 9, semantics.Symbols[0].EndLine)
	assert.Equal(t, "cmd_spawn", semantics.Symbols[1].Name)
	assert.Equal(t, "function", semantics.Symbols[1].Kind)
	assert.Equal(t, "main", semantics.Symbols[2].Name)
	assert.Empty(t, semantics.Tests)
	require.Len(t, semantics.Imports, 1)
	assert.Equal(t, "$ROOT/lib/core.sh", semantics.Imports[0].Name)
	assert.Equal(t, 2, semantics.Imports[0].Line)
	assert.Equal(t, 2, semantics.Imports[0].EndLine)
}

func TestExtractShellSemantics_WithBatsTests_ExtractsFormalAndDynamicCases(t *testing.T) {
	body := `@test "addition works" {
  true
}

dynamic_case() {
  true
}
bats_test_function --description "dynamic behavior" -- dynamic_case
`

	semantics, err := ExtractShellSemantics("test/math.bats", "test", body)

	require.NoError(t, err)
	assert.Empty(t, semantics.Symbols)
	require.Len(t, semantics.Tests, 2)
	assert.Equal(t, "addition works", semantics.Tests[0].Name)
	assert.Equal(t, "bats_test", semantics.Tests[0].Kind)
	assert.Equal(t, 1, semantics.Tests[0].Line)
	assert.Equal(t, 3, semantics.Tests[0].EndLine)
	assert.Equal(t, "dynamic behavior", semantics.Tests[1].Name)
	assert.Equal(t, "dynamic_case", semantics.Tests[1].Parent)
	assert.Equal(t, 8, semantics.Tests[1].Line)
}

func TestExtractShellSemantics_WithVerificationScript_UsesBehaviorSectionsInsteadOfFakeFunctions(t *testing.T) {
	body := `#!/usr/bin/env bash
# Verification checkpoint rejects a stale baseline.
# The lane remains available for inspection.
false
fake_provider() { :; }
# Selection gate preserves explicit provider choice.
true
`

	semantics, err := ExtractShellSemantics("scripts/verify.sh", "test", body)

	require.NoError(t, err)
	assert.Empty(t, semantics.Symbols)
	require.Len(t, semantics.Tests, 2)
	assert.Equal(t, "Verification checkpoint rejects a stale baseline. The lane remains available for inspection.", semantics.Tests[0].Name)
	assert.Equal(t, "shell_behavior", semantics.Tests[0].Kind)
	assert.Equal(t, 2, semantics.Tests[0].Line)
	assert.Equal(t, 5, semantics.Tests[0].EndLine)
	assert.Equal(t, "Selection gate preserves explicit provider choice.", semantics.Tests[1].Name)
	assert.Equal(t, 6, semantics.Tests[1].Line)
	assert.Equal(t, 8, semantics.Tests[1].EndLine)
}

func TestExtractShellSemantics_WithInvalidVerificationScript_RetainsWholeFileTestAnchor(t *testing.T) {
	body := "#!/usr/bin/env bash\nif then\n"

	semantics, err := ExtractShellSemantics("scripts/verify.sh", "test", body)

	require.Error(t, err)
	assert.Empty(t, semantics.Symbols)
	assert.Empty(t, semantics.Imports)
	require.Len(t, semantics.Tests, 1)
	assert.Equal(t, "verify", semantics.Tests[0].Name)
	assert.Equal(t, "shell_test", semantics.Tests[0].Kind)
	assert.Equal(t, 1, semantics.Tests[0].Line)
	assert.Equal(t, 3, semantics.Tests[0].EndLine)
}

func TestExtractShellSemantics_WithEscapedDotCommand_ExtractsSourcedFile(t *testing.T) {
	body := "#!/bin/sh\n\\. ../../../nvm.sh\n"

	semantics, err := ExtractShellSemantics("test/fast/validation", "test", body)

	require.NoError(t, err)
	require.Len(t, semantics.Imports, 1)
	assert.Equal(t, "../../../nvm.sh", semantics.Imports[0].Name)
	assert.Equal(t, 2, semantics.Imports[0].Line)
	assert.Equal(t, 2, semantics.Imports[0].EndLine)
}
