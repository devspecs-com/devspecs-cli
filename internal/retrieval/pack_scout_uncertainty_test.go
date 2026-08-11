package retrieval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyScoutUncertaintyForQueryWarnsOnThinSourceAndMissingAnchor(t *testing.T) {
	pack := RoleGroupedPack{
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 9, Path: "src/textual/selection.py", Title: "selection.py", Role: PackRoleImplementation},
				},
			},
			{
				Role: PackRoleBehaviorTests,
				Items: []PackItem{
					{OriginalRank: 1, Path: "tests/text_area/test_selection_bindings.py", Title: "selection bindings", Role: PackRoleBehaviorTests, Subtype: "test_case"},
					{OriginalRank: 2, Path: "tests/selection_list/test_over_wide_selections.py", Title: "wide selections", Role: PackRoleBehaviorTests, Subtype: "test_case"},
					{OriginalRank: 3, Path: "tests/text_area/test_selection.py", Title: "selection", Role: PackRoleBehaviorTests, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyScoutUncertaintyForQuery(pack, "Fix selection disappearing")
	require.Equal(t, "true", got.Metadata[PackScoutUncertaintyKey],
		"missing uncertainty metadata: %#v", got.Metadata)

	reasons := got.Metadata[PackScoutUncertaintyReasonsKey]
	require.True(t, strings.Contains(reasons, "implementation surface is thin"),
		"missing thin source warning: %q", reasons)
	require.True(t, strings.Contains(reasons, "disappear"),
		"missing query-anchor warning: %q", reasons)

}

func TestApplyScoutUncertaintyForQuerySkipsBalancedSourceTestPack(t *testing.T) {
	pack := RoleGroupedPack{
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 1, Path: "discovery/aws/rds.go", Title: "rds discovery", Role: PackRoleImplementation},
				},
			},
			{
				Role: PackRoleBehaviorTests,
				Items: []PackItem{
					{OriginalRank: 2, Path: "discovery/aws/rds_test.go", Title: "rds discovery test", Role: PackRoleBehaviorTests, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyScoutUncertaintyForQuery(pack, "Handle RDS clusters without instances in AWS discovery")
	if got.Metadata != nil {
		assert.NotEqual(t, "true", got.Metadata[PackScoutUncertaintyKey])
	}

}

func TestApplyScoutUncertaintyForQueryDoesNotWarnWhenSingleTestSupportsBroadSourcePack(t *testing.T) {
	pack := RoleGroupedPack{
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 1, Path: "httpie/manager/tasks/sessions.py", Role: PackRoleImplementation},
					{OriginalRank: 2, Path: "httpie/legacy/v3_1_0_session_cookie_format.py", Role: PackRoleImplementation},
					{OriginalRank: 3, Path: "httpie/legacy/v3_2_0_session_header_format.py", Role: PackRoleImplementation},
					{OriginalRank: 4, Path: "httpie/sessions.py", Role: PackRoleImplementation},
				},
			},
			{
				Role: PackRoleBehaviorTests,
				Items: []PackItem{
					{OriginalRank: 5, Path: "tests/test_sessions.py", Role: PackRoleBehaviorTests, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyScoutUncertaintyForQuery(pack, "upgrade old HTTPie session files and bind legacy cookies to the current host")
	if got.Metadata != nil {
		assert.NotEqual(t, "true", got.Metadata[PackScoutUncertaintyKey])
	}

}

func TestApplyScoutUncertaintyForQueryWarnsWhenSingleTestFamilyLacksSourceSupport(t *testing.T) {
	pack := RoleGroupedPack{
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 1, Path: "crates/ty_python_semantic/src/types/function.rs", Role: PackRoleImplementation},
					{OriginalRank: 2, Path: "crates/ty_python_semantic/src/types/infer/builder/function.rs", Role: PackRoleImplementation},
					{OriginalRank: 3, Path: "crates/ruff_linter/src/rules/flake8_return/rules/function.rs", Role: PackRoleImplementation},
				},
			},
			{
				Role: PackRoleBehaviorTests,
				Items: []PackItem{
					{OriginalRank: 4, Path: "crates/ty_server/tests/e2e/completions.rs", Role: PackRoleBehaviorTests, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyScoutUncertaintyForQuery(pack, "[ty] Add function parentheses completion")
	require.Equal(t, "true", got.Metadata[PackScoutUncertaintyKey],
		"missing uncertainty metadata: %#v", got.Metadata)

	reasons := got.Metadata[PackScoutUncertaintyReasonsKey]
	require.True(t, strings.Contains(reasons, "visible behavior test is not backed"),
		"missing source-family gap warning: %q", reasons)
	require.True(t, strings.Contains(reasons, "completion"),
		"missing test family root in warning: %q", reasons)

}
