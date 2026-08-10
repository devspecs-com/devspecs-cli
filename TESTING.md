# Testing

DevSpecs uses the standard `testing` package for test structure and Testify
for assertions. Keep tests as plain functions and table-driven subtests; do not
introduce Testify suites.

## Assertions

Use `require` when later statements depend on the assertion:

- setup, filesystem, database, command execution, and decoding errors;
- expected errors before inspecting their message or type;
- lengths before indexing and non-empty values before slicing;
- type assertions before dereferencing the asserted value.

Use `assert` for independent comparisons so sibling failures remain visible.
Prefer semantic helpers such as `Equal`, `ElementsMatch`, `JSONEq`,
`ErrorContains`, `Contains`, and `NotContains` over `True` with a compound
boolean expression.

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

Run the aggregate report with `make cover`. Coverage changes must exercise
observable behavior or failure handling; do not add production abstractions or
empty calls solely to increase the percentage. The hardening track starts at
78.04% and targets a stable CI floor of at least 80%.
