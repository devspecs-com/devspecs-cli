# False-positive samples

Curated inputs where markdown checklist extraction is **syntactically correct** but **semantically unwanted**: documentation or examples that use real `- [ ]` / `- [x]` / `- [X]` lines without intending them as project todos.

Implementation under test: [`internal/adapters/todoparse`](../../../internal/adapters/todoparse).

## `todoparse/`

| Fixture | Real-world analogue |
|--------|----------------------|
| [`todoparse/supported-syntax-examples.md`](todoparse/supported-syntax-examples.md) | [`v0.spec.md`](../../../v0.spec.md) — section *Explicit Todo Extraction*, *Supported syntax* (~lines 1246–1250). `ds todos` on that artifact reports an open item for the illustrative `- [ ] Incomplete task` line. |

### Workaround for authors

Wrap illustrative checklists in a **fenced** block (for example ` ```markdown ` … ` ``` `). The parser skips fenced regions; see `TestTodoParser/inside_fenced_code_block_ignored` in [`todoparse_test.go`](../../../internal/adapters/todoparse/todoparse_test.go).

### Future parser work (non-binding)

Possible heuristics: headings like “Supported syntax” / “Example”, or other doc-context signals. When behavior changes, update the baseline test that reads these fixtures.
