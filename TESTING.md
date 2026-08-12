# Testing

DevSpecs uses the standard `testing` package for test structure and Testify
for assertions. Keep each scenario in one plain test function beside the
implementation in the matching `_test.go` file. Do not introduce Testify
suites, table-driven tests, test-case loops, or multi-case subtests.

## Structure

Give tests descriptive names that identify the subject, condition, and expected
behavior, such as `TestCompareCounts_WhenCountIncreases_ReturnsRegression`.
Keep one behavior in each test and separate Arrange, Act, and Assert with blank
lines. A test has one Act phase: prepare the complete starting state, invoke the
subject once, and then verify the result. When two invocations represent two
states or transitions, write two tests and arrange each required starting state
directly.

Prefer real temporary directories, SQLite databases, and `httptest.Server` over
mocks. When a seam is necessary, keep the fake minimal and local to the test.
Helpers should only remove incidental setup or observation noise; they must not
hide the behavior under test or combine scenarios.

## Assertions

Use `require` when later statements depend on the assertion:

- setup, filesystem, database, command execution, and decoding errors;
- expected errors before inspecting their message or type;
- lengths before indexing and non-empty values before slicing;
- type assertions before dereferencing the asserted value.

Use `assert` for independent comparisons so sibling failures remain visible.
Prefer semantic helpers such as `Equal`, `JSONEq`, `ErrorContains`, `Contains`,
and `NotContains` over `True` with a compound boolean expression.

Do not compare an expected slice with an actual slice in one assertion. Assert
`Nil` for a nil slice, `Empty` for an empty slice, or establish a non-empty
slice's shape with `require.Len` before asserting each index independently.
Apply the same principle to maps: establish the size, then assert values by key.
This preserves both safe indexing and precise failure output.

```go
func TestCompareCounts_WhenCountIncreases_ReturnsRegression(t *testing.T) {
	baseline := map[string]int{"sample_test.go": 2}
	current := map[string]int{"sample_test.go": 3}

	problems := compareCounts(current, baseline)

	require.Len(t, problems, 1)
	assert.Equal(t, "sample_test.go: direct testing assertions increased from 2 to 3", problems[0])
}
```

For stateful behavior, arrange the state needed by the single invocation under
test. Do not assert one invocation and then invoke the subject again in the same
test.

```go
func TestResolveRepo_WithExistingGitIdentity_ReusesRepository(t *testing.T) {
	db := openTestDB(t)
	existingID := insertRepository(t, db, repositoryFixture{
		GitRemote: "https://github.com/devspecs-com/devspecs-cli.git",
		RootCommit: "abc123",
	})
	identity := RepoIdentity{
		RootPath:  "/tmp/another-worktree",
		GitRemote: "https://github.com/devspecs-com/devspecs-cli.git",
		RootCommit: "abc123",
	}

	actualID, err := ResolveRepo(db, identity)

	require.NoError(t, err)
	assert.Equal(t, existingID, actualID)
}
```

Do not call `require` from a test goroutine because it may invoke `FailNow`.
Send errors or results back to the test goroutine and assert there. Literal Go
snippets used as scanner fixtures are not assertion debt.

## Assertion Debt

The migration from direct `t.Fatal`, `t.Fatalf`, `t.Error`, and related calls
is ratcheted by a syntax-aware baseline:

```bash
go run ./scripts/ci/check-test-assertions
```

After reviewing an intentional reduction, update the baseline in the same
change:

```bash
go run ./scripts/ci/check-test-assertions --update
```

The final migration target is an empty baseline, apart from any explicitly
documented exception that cannot safely use Testify.

## Coverage

Run the aggregate report with `make cover` and enforce the release floor with
`make cover-check`. Local and CI checks use exact covered/total statement counts
from `coverage.out`; rounded `go tool cover` display percentages are not used for
the decision. The aggregate floor is 80.0%, and CI combines it with `-race` and
atomic coverage mode.

Coverage changes must exercise observable behavior or failure handling. Do not
add production abstractions or empty calls solely to increase the percentage.
