package retrieval

import (
	"strings"
	"testing"
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
	if got.Metadata[PackScoutUncertaintyKey] != "true" {
		t.Fatalf("missing uncertainty metadata: %#v", got.Metadata)
	}
	reasons := got.Metadata[PackScoutUncertaintyReasonsKey]
	if !strings.Contains(reasons, "implementation surface is thin") {
		t.Fatalf("missing thin source warning: %q", reasons)
	}
	if !strings.Contains(reasons, "disappear") {
		t.Fatalf("missing query-anchor warning: %q", reasons)
	}
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
	if got.Metadata != nil && got.Metadata[PackScoutUncertaintyKey] == "true" {
		t.Fatalf("balanced source/test pack should not warn: %#v", got.Metadata)
	}
}
