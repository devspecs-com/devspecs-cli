# ADR 017: Use Taskfile as the project task runner

## Status

Accepted

## Context

Developer workflows involve many commands: running tests, linting, formatting, building, cleaning, deploying, etc. Without a task runner, contributors must remember tool-specific invocations (`hatch run test`, `hatch run lint`, `ruff format src/ tests/`) or hunt through documentation.

Options considered:

| Option                         | Pros                                                               | Cons                                                |
| ------------------------------ | ------------------------------------------------------------------ | --------------------------------------------------- |
| **Makefile**                   | Ubiquitous on Unix, well-known                                     | Poor Windows support, tab-sensitivity, shell quirks |
| **Just**                       | Simple syntax, cross-platform                                      | Less mature ecosystem, fewer features               |
| **Nox / tox**                  | Python-native, good for test matrices                              | Heavier, overlaps with Hatch's env management       |
| **npm scripts / package.json** | Familiar to JS developers                                          | Wrong ecosystem for a Python project                |
| **Taskfile**                   | YAML syntax, true cross-platform, variables, prompts, dependencies | Less well-known than Make                           |
| **Shell scripts**              | No extra tool needed                                               | Not cross-platform, scattered across files          |

### Why not just use Hatch scripts directly?

Hatch scripts (`hatch run <script>`) are excellent for tool invocations but lack features like:

- Task dependencies (run lint _before_ test)
- Confirmation prompts for destructive operations
- Grouping non-Hatch commands (e.g., `sqlite3`, `gh`, git operations)
- Discoverability via `task --list`

Taskfile wraps Hatch commands where appropriate and adds project-level orchestration on top.

## Decision

Use [Taskfile](https://taskfile.dev/) (`Taskfile.yml`) at the project root as the primary developer interface for all routine commands. Taskfile delegates to Hatch for Python-specific operations (test, lint, format, typecheck) and runs other tools directly where needed.

Key conventions:

- **Namespaced tasks** — `test:unit`, `test:cov`, `lint:fix`, `clean:all`, etc.
- **`CLI_ARGS` passthrough** — tasks accept extra arguments via `-- --flag` for flexibility
- **Dedicated shortcuts** — common operations have hardcoded tasks (e.g., `task labels:baseline`) to avoid the `--` syntax
- **Confirmation prompts** — destructive tasks like `db:reset` and `env:prune` require confirmation

## Consequences

### Positive

- Single entry point for all project commands — `task --list` shows everything
- Cross-platform — works identically on Windows, macOS, and Linux
- Low overhead — Taskfile is a single static binary with no runtime dependencies
- Wraps Hatch without replacing it — developers who prefer Hatch directly can still use it
- Self-documenting — each task has a `desc` field shown in the task list

### Negative

- Adds a tool dependency (Taskfile must be installed separately)
- Developers must learn `task` CLI conventions (e.g., `--` for argument passthrough)
- Taskfile must be kept in sync with Hatch scripts and project tooling

### Neutral

- Taskfile is optional — all underlying commands work without it
- CI uses Hatch directly rather than Taskfile, keeping CI independent of the task runner
