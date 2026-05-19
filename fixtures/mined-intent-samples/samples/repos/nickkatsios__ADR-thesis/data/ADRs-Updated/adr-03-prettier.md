# ADR 3 - Code Formatting

## Context

Code formatters programmatically apply consistent formatting to a codebase to ensure
consistent style. The intended goals are improving readability and reducing bikeshedding
over code style.

For JavaScript, this trend may have started with `eslint --fix` but gained momentum with
[Prettier](https://prettier.io/). This trend is not specific to JavaScript (consider gofmt
and rustfmt).

At Guidewire:

- The InsuranceNow code base has recently adopted autoformatting
- The Enterprise pod uses Prettier extensively

## Decision

We will use Prettier as a precommit git hook. Developers can also install IDE plugins to
apply Prettier on file save.

## Status

Adopted. However, the Prettier hooks could be removed with minimal effort.

## Consequences

- The code committed may differ stylistically from the author's original input.
- The codebase has consistent styling.
