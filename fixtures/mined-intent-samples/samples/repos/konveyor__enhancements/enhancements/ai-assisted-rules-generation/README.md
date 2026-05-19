# AI-Assisted Rules Generation

Extract migration patterns from documentation and generate structurally valid Konveyor rules using skills. 

## Release Signoff Checklist

- Enhancement is `implementable`
- Design details are appropriately documented from clear requirements
- Test plan is defined
- User-facing documentation is created

## Open Questions

1. **Test data generation validity**: LLM-generated test data is biased toward the LLM's understanding of the rule. Is synthetic test data useful beyond smoke testing?
2. **Scaling to large guides**: Very large migration guides could produce hundreds of patterns. Are there token limit concerns or chunking issues at that scale?
3. **Cross-model quality variance**: Skills are Markdown instructions interpreted by different LLMs. How much does output quality vary across models (Claude vs GPT-4 vs Gemini)? What's the minimum model capability needed for reliable results?
4. **Non-deterministic extraction**: Pattern extraction is LLM-driven and inherently non-deterministic. The same migration guide can produce different patterns across runs -- different counts, different granularity, different condition type choices. How do we establish confidence in extraction completeness? What's the acceptable variance threshold?

## Summary

Konveyor supports multiple languages, but only Java has a substantial ruleset. Creating rules requires both
domain-specific migration knowledge and Konveyor rule syntax expertise, which is a high barrier that limits adoption. Konveyor and Kai are only useful if rules exist to support the migration. Even for custom libraries within enterprises,custom rules are needed.

This enhancement proposes a skill that extracts migration patterns from documentation (migration guides, changelogs, code snippets) and generates structurally valid Konveyor rules. It has:

- **Agent Skills** (`agents/`) -- 4 Markdown-based skill files that orchestrate the full pipeline including LLM-powered workflows (pattern extraction, test code generation, test data repair). These follow the [AgentSkills.io](https://agentskills.io) open standard and work with any compatible agent runtime (Claude Code, Cursor, Goose, etc.). Skills are the primary interface -- users interact with them through their agent runtime.
- **Go Helpers** (`cmd/`) -- Helper functions that skills invoke for deterministic operations: ingestion, rule construction, validation, test scaffolding, XML sanitization, result stamping, and reporting. These are not user-facing commands -- they are internal tools called by the skills via `go run ./cmd/<name>`.

The Go helpers have zero LLM dependency and can be tested and validated independently; the LLM's job is limited to understanding natural language (migration guides) and generating code (test data); and any agent runtime that can read Markdown and execute shell commands can run the full pipeline.

Generated rules carry verification labels (`konveyor.io/generated-by`, `konveyor.io/test-result`, `konveyor.io/review`) that track provenance and verification status through the pipeline. 

## Motivation

### The Rules Gap

- Only Java has comprehensive rulesets (inherited from Windup)
- Go, Node.js, C#, Python, and frontend frameworks have minimal coverage
- Enterprise-specific rules (custom libraries, internal frameworks) don't exist
- Creating rules requires understanding both the migration domain AND Konveyor's YAML rule syntax

### Goals

- **Lower the barrier to creating rules**: A developer with migration knowledge but no Konveyor syntax expertise should be able to generate rules
- **Expand language/framework coverage**: Enable rule generation for any language or framework where migration
documentation exists
- **Verify AI-generated rules**: Validate rules structurally (syntax) and functionally (kantra)
- **Integrate with the Konveyor ecosystem**: Generated rules work with kantra, Kai, and compatible with existing rulesets

### Non-Goals

- **Replace human rule authors**: AI generates draft rules; humans review and decide whether to commit
- **Guarantee rule correctness**: AI-generated rules are proposals, not verified facts
- **Build a new rule engine**: Rules use existing Konveyor YAML format, validated by existing kantra infrastructure
- **Define Hub or enterprise rule distribution**: How generated rules are attached to analysis profiles, custom migration targets, or rolled out inside an organization is out of scope. This enhancement outputs standard Konveyor YAML; consumption after publication uses existing platform paths.

The **intended upstream path** for community rules is **human review and a pull request to [konveyor/rulesets](https://github.com/konveyor/rulesets)**, as in the user stories below. Staging layers (for example rule-candidate flows in the Kai solution server), Hub-specific packaging, or org-wide distribution workflows are **not specified here**; they may be addressed by other enhancements or existing Konveyor features.

## Proposal

### User Stories

#### Story 1: Generate rules from a migration guide

A developer needs Konveyor rules for migrating Spring Boot 3 to 4. They have the official migration guide URL. They invoke the `generate-rules` agent skill, which ingests the guide, delegates to the `rule-writer` skill to extract migration patterns and construct rules, then optionally delegates to `test-generator` for test data and runs validation via `cmd/test`. If tests fail, the `rule-validator` skill fixes the test data. The developer reviews and submits a PR to konveyor/rulesets.

#### Story 2: Agentic workflow via Agent Skill

A developer invokes the `generate-rules` skill in their agent runtime (Claude Code, Goose, Cursor, etc.), providing a migration guide URL as the argument. If no argument is provided, the skill asks for the migration guide source.

The orchestrator skill coordinates the full pipeline: ingest the guide, delegate to `rule-writer` for pattern extraction and rule construction, then ask the developer whether to run tests. If yes, it delegates to `test-generator` and `rule-validator`, runs a fix loop on failures (up to 3 iterations), and finalizes by stamping verification labels and generating a summary report.

#### Story 3: Expand coverage for a new language

A contributor wants to add Go rulesets to konveyor/rulesets. They gather migration guides for popular Go frameworks and libraries (e.g., Go 1.x to 1.y standard library changes, gRPC version upgrades, popular router migrations). They run the `generate-rules` skill against each guide to bootstrap a set of validated rules, then submit PRs to konveyor/rulesets for community review.


### Implementation Details/Notes/Constraints

#### Architecture

<!-- stripped fenced code block: text -->

#### Agent Skills

The skills are the primary interface. Users interact with the skills through their agent runtime; the skills orchestrate the entire workflow and invoke Go helper functions for deterministic operations.

Four agent skills in `agents/`, each a Markdown file with a `SKILL.md` and optional `references/` directory, following the [AgentSkills.io](https://agentskills.io) format.

##### Skill Composition via Sub-agents

The `generate-rules` orchestrator delegates LLM-heavy work to sub-skills using **invoke blocks** -- declarative sections in the skill Markdown that name a sub-skill, pass inputs, and state expected outputs. The agent runtime translates each invoke block into a sub-agent call by instructing it to "read and follow `agents/<skill-name>/SKILL.md`" with the provided inputs. Invoke blocks marked `Parallel: yes` can be dispatched concurrently if the runtime supports it.

The primary motivation is **context management**: each sub-agent runs in its own context window with only its skill instructions and reference docs loaded. Without sub-agents, the orchestrator's context would accumulate the full migration guide, all reference documentation (condition types, rule schema, provider-specific fix strategies), extracted patterns, generated code, and kantra output -- easily exceeding context limits on large guides. Sub-agents keep the orchestrator's context lean (pipeline state and results only) while each sub-agent gets a focused context for its specific task.

This approach is also runtime-agnostic: any agent runtime that can read Markdown, spawn sub-agents, and execute shell commands can run the full pipeline. The skills contain no runtime-specific APIs or SDK calls.

##### `generate-rules` (orchestrator)

The top-level pipeline orchestrator. Coordinates the full end-to-end flow across 5 steps:

1. Ingest the migration guide
2. Delegate to `rule-writer` to extract patterns and construct rules
3. Optionally delegate to `test-generator` to create test data
4. Run tests via `cmd/test` in batched sequential runs
5. Delegate to `rule-validator` to fix failing test data (up to 3 iterations), finalize with stamping labels and reporting

The orchestrator asks the user whether to run tests or skip them.

##### `rule-writer`

Reads the migration guide content and extracts every migration pattern into a structured `patterns.json` file (the `ExtractOutput` format). It:

- Auto-detects source, target, and language from the guide content
- Deduplicates patterns across sections
- Generates migration messages with before/after code examples
- Invokes the `construct` helper to build rule YAML from patterns
- Invokes the `validate` helper to check structural correctness

The skill includes extensive reference documentation on all 12 condition types, the `patterns.json` schema, rule YAML schema, and working examples for languages we support.

##### `test-generator`

Creates compilable test projects that trigger each rule's `when` condition. It:

- Invokes the `scaffold` helper to create the test directory structure and `manifest.json`
- Reads the manifest to know exactly which files to generate (build file + source file per test group)
- Generates language-appropriate source code that triggers rule conditions
- Resolves dependencies if needed
- Invokes the `sanitize` helper to fix bad XML comments that LLMs may generate

##### `rule-validator`

Fixes failing test data so rules pass kantra validation. Uses a reference-driven approach: reads each failing rule's condition type, looks up the known fix from language-specific provider references (`references/providers/java.md`, `go.md`, `nodejs.md`, `csharp.md`), and applies targeted fixes. Falls back to general LLM reasoning for failure modes not covered by the references. Verifies fixes by running tests via `cmd/test` and iterates up to `max_iterations` (default 1, max 3).

Enforces a strict **rule integrity principle**: never change rules to fix tests; always fix the test data.

#### Helpers

Eight Go helper functions in `cmd/`, invoked by the skills via `go run ./cmd/<name>`. These are internal tools, not user-facing commands. All helpers have zero LLM dependency and can be tested independently.


| Helper     | Purpose                                                                                         |
| ---------- | ----------------------------------------------------------------------------------------------- |
| `ingest`   | Fetch migration guide from URL (with HTML-to-Markdown conversion), file, or raw text            |
| `construct`| Convert `patterns.json` into Konveyor rule YAML files grouped by concern                        |
| `validate` | Check rule YAML structural correctness (required fields, condition-specific requirements)        |
| `scaffold` | Create kantra test directory structure, `.test.yaml` files, and `manifest.json` from rules      |
| `sanitize` | Fix bad XML comments (`--` inside `<!-- -->`) that LLMs frequently generate                     |
| `test`     | Run `kantra test` per `.test.yaml` file, return structured JSON pass/fail results               |
| `stamp`    | Update rule YAML labels with `konveyor.io/test-result=passed` or `failed` after test runs       |
| `report`   | Generate YAML summary report with rule counts, test pass rate, and failing rule IDs             |

##### Data Flow

```text
1. Input (URL/file/text)
   └─> cmd/ingest ──> guide.md (clean Markdown)

2. Agent reads guide.md
   └─> rule-writer skill extracts patterns ──> patterns.json (ExtractOutput)

3. patterns.json
   └─> cmd/construct ──> rules/*.yaml + ruleset.yaml

4. rules/*.yaml
   └─> cmd/validate ──> validation result (pass/fail JSON)

5. rules/*.yaml
   └─> cmd/scaffold ──> tests/*.test.yaml + tests/data/ + manifest.json

6. Agent reads manifest.json
   └─> test-generator skill creates source code in tests/data/

7. cmd/sanitize ──> cleans XML files in tests/data/

8. cmd/test ──> runs kantra per .test.yaml, returns JSON pass/fail results

9. Fix loop (up to 3 iterations):
   └─> rule-validator fixes test data ──> cmd/test re-verifies

10. cmd/stamp ──> updates rule labels with pass/fail
    cmd/report ──> report.yaml summary
```

##### Key Intermediate Format: `patterns.json`

The `patterns.json` file (`ExtractOutput` format) is the contract between the LLM (agent) and the deterministic Go helpers. It contains:

- `source`, `target`, `language` (top-level metadata)
- `patterns[]` array of `MigrationPattern` objects with 20+ fields covering:
  - Source/target patterns and FQNs
  - Location types and provider types
  - File patterns and dependency info
  - XPath expressions
  - Complexity and category
  - Concern grouping
  - Documentation URLs
  - Before/after code examples
  - Migration messages

The helpers handle all mechanical transformation from this intermediate format to valid Konveyor rule YAML. This separation enables extract-once/construct-many iteration, auditability (JSON artifacts can be inspected and modified between stages), tool composition (pipe extract output through external transforms), and deterministic replay (same JSON always produces the same rules).

##### Verification Labels

Generated rules carry metadata labels that track provenance and verification status:


| Label                      | Values                               | Description                          |
| -------------------------- | ------------------------------------ | ------------------------------------ |
| `konveyor.io/generated-by` | `ai-rule-gen`                        | Marks rules as AI-generated          |
| `konveyor.io/test-result`  | `untested`, `passed`, `failed`       | Updated by `stamp` after kantra runs |
| `konveyor.io/review`       | `unreviewed`, `approved`, `rejected` | For human review workflow            |


Labels are initialized when rules are constructed, stamped with test results after kantra runs via `cmd/stamp`, and queryable for filtering. This enables CI pipelines to filter rules by verification status (e.g., only commit rules with `test-result=passed`).

##### Supported Condition Types


| Language           | Condition Types                                                                                                |
| ------------------ | -------------------------------------------------------------------------------------------------------------- |
| Java               | `java.referenced`, `java.dependency`                                                                           |
| Go                 | `go.referenced`, `go.dependency`                                                                               |
| Node.js/TypeScript | `nodejs.referenced`                                                                                            |
| C#                 | `csharp.referenced`                                                                                            |
| Any                | `builtin.filecontent`, `builtin.file`, `builtin.xml`, `builtin.json`, `builtin.hasTags`, `builtin.xmlPublicID` |


Each condition type has specific fields. Conditions can be combined with `or` and `and` combinators.

#### Agentic Workflow

**Orchestrated skill workflow** (Story 2: agentic workflow):

<!-- stripped fenced code block: text -->

#### Future Considerations

##### Language-Specific Rules Generation Skills

The current skills are language-agnostic -- the same `rule-writer` skill handles Java, Go, Node.js, and C# by relying on the reference documentation for each condition type. As rule generation matures, language-specific skills could provide deeper, more accurate pattern extraction:

- **Language-specific rule-writer skills**: A `java-rule-writer` skill with deep knowledge of Java ecosystem conventions (Maven/Gradle dependencies, annotation processing, servlet APIs, EJB patterns) could extract patterns that a generic skill misses. Similarly for `go-rule-writer`, `nodejs-rule-writer`, and `csharp-rule-writer`.
- **Language-specific test-generator skills**: A `java-test-generator` could generate more realistic Java test projects with proper Maven/Gradle structure, dependency management, and framework-specific boilerplate. A `go-test-generator` would handle Go module conventions, vendoring, and build constraints natively.
- **Skill composition**: The `generate-rules` orchestrator could auto-detect the target language and delegate to the appropriate language-specific skill, falling back to the generic skill when no specialized one exists.
- **Community-contributed skills**: The AgentSkills.io format makes it easy for domain experts to contribute language or framework-specific skills without writing Go code. A Quarkus expert could write a `quarkus-migration` skill; a .NET expert could write a `dotnet-migration` skill.

This approach keeps the core architecture unchanged -- language-specific skills still invoke the same Go helpers -- while allowing the LLM-driven layer to become increasingly specialized.

#### Existing Work

- **[konveyor/rulesets](https://github.com/konveyor/rulesets)** — this repo contains curated rules. We do not change layout or governance here; we aim to lower the cost of producing reviewable candidates that still go through normal review and PR.
- **[analyzer-lsp MCP](https://github.com/konveyor/enhancements/pull/259)** (analyzer engine via MCP) — this pr emphasizes **running analysis** (`analyze_run`, `rules_validate`, `dependencies_get`, etc.). Current enhancement focusses on **authoring** rules. generated rules should still validate and run through the same analyzer stack.
- **[analyzer-rule-generator](https://github.com/konveyor-ecosystem/analyzer-rule-generator)** — Closest prior art (Python, migration guides, Claude-oriented flows). We overlap on “guide in, rules out” but diverge on stack: AgentSkills.io + Go `cmd/` helpers, with **`patterns.json` as the stable contract** so deterministic steps stay testable without standardizing on one Python deployment.
- **[scribe](https://github.com/sshaaf/scribe)** (Java MCP server for rules) — Overlap in “MCP + rules,” but we are not trying to own a general rule CRUD server in this enhancement; MCP remains optional alongside batch and agent workflows.
- **[semver-analyzer](https://github.com/shawn-hurley/semver-analyzer)** — deterministic API diff in Rust. We can treat it as a **possible future signal** (semver/API changes feeding extraction) rather than something this proposal implements.

### Security, Risks, and Mitigations

- **Prompt injection**: Migration guides from URLs could contain adversarial content. The agent skills include structured extraction formats that constrain LLM output.
- **URL ingestion**: URLs are fetched with a hardened HTTP client with SSRF mitigation that blocks loopback and private IP addresses.
- **File system access**: Path traversal is restricted. Workspace directory names are sanitized (no `..`, `/`, `\`).
- **Supply chain**: Generated rules affect how kantra analyzes applications. Human review before committing to rulesets is essential.
- **LLM hallucination**: Structural validation via `cmd/validate` catches invalid regex, wrong condition types, and missing dependency version bounds. Verification labels and rules reports provide transparency into what has been tested. Semantic errors require human review.

## Design Details

### Test Plan

#### Go Helper Tests

- **Unit tests** (`go test ./internal/...`): Core library functions (ingestion, rule construction, condition builders, validation, serialization, labels, workspace, scaffolding, kantra parsing, sanitization). No LLM or kantra needed.

#### Agent Skill Evals

Skills are LLM-driven and non-deterministic, so they require evaluation-based testing rather than traditional unit tests. Evals measure skill quality across runs and across models.

**Eval approach**:

- **Golden-input evals**: Run each skill against known migration guides with expected outputs. Compare generated `patterns.json`, rule YAML, and test data against reference baselines. Score on: pattern coverage (did it find all known patterns?), rule validity (do all rules pass `cmd/validate`?), and test pass rate (do generated tests pass kantra?).
- **Per-skill evals**:
  - `rule-writer`: Given a migration guide, measure pattern extraction recall/precision against a curated pattern list. Check that `patterns.json` is valid and `cmd/construct` succeeds.
  - `test-generator`: Given rules, measure whether generated test code compiles and triggers the expected rule conditions in kantra.
  - `rule-validator`: Given rules with known test failures, measure whether the agent correctly identifies condition types, applies the right fix from provider references, and produces passing test data.
  - `generate-rules` (orchestrator): End-to-end eval from migration guide URL to final report. Measure total rule count, test pass rate, and fix loop convergence.
- **Cross-model evals**: Run the same eval suite across different agent runtimes and models (Claude Sonnet, Claude Opus, GPT-4o, Gemini) to identify model-specific regressions and establish minimum quality baselines.
- **Regression evals**: On each skill change, re-run the eval suite to catch quality regressions. Track metrics over time to measure improvement.

**Eval infrastructure**: Evals are intended to run as CI jobs that invoke the skills headlessly (via an agent runtime in non-interactive mode), collect outputs, and score them against baselines. 

### Upgrade / Downgrade Strategy

**Upgrade**: Skills are Markdown files -- upgrading is replacing the skill files in `agents/`. No migration needed. Go helpers are standalone binaries with no persistent state. Generated rules use the standard Konveyor YAML format and work with any version of kantra that supports the condition types used.

**Downgrade**: No persistent state -- no databases, caches, or config files. Reverting to older skill or helper versions has no side effects. Generated output files are plain YAML and do not depend on the generating version.

**Skill versioning**: Skills evolve independently of the Go helpers. Since skills are Markdown instructions interpreted by the agent runtime, changes to skill wording can affect output quality without any code changes. The eval suite (above) serves as the regression gate for skill changes -- a skill update that degrades eval scores should not ship.

**Helper compatibility**: The `patterns.json` schema is the contract between skills and helpers. Breaking changes to this schema require coordinated updates to both the `rule-writer` skill (which produces it) and `cmd/construct` (which consumes it).

## Implementation History

## Drawbacks

- **LLM dependency for extraction**: Pattern extraction requires an LLM, provided by the agent's own model. Quality varies by agent and model. Mitigation: the `patterns.json` intermediate format allows human inspection and correction before rule construction.
- **Validation gap**: No fully trustworthy automated validation for AI-generated rules. Synthetic test data is biased. Human review remains necessary.
- **Agent dependency**: The skill-first approach requires an agent runtime to run the full pipeline. Users without an agent runtime can still invoke the Go helpers directly with manually-created `patterns.json` files.

## Alternatives

### Single Binary with Embedded LLM Support

A monolithic Go binary (`rulegen`) with built-in LLM provider support (Anthropic, OpenAI, Gemini, Ollama) and user-facing CLI commands like `generate`, `extract`, `test`, `pipeline`.

- **Pros**: Self-contained. Works without an agent runtime. Single binary distribution. Direct CLI usage in CI/CD.
- **Cons**: Requires API key management. LLM provider maintenance burden. Prompt templates embedded in Go code. Less flexible than agent-native extraction.

### MCP Server

MCP server exposing deterministic tools (`construct_rule`, `validate_rules`, `get_help`) for IDE integration.

- **Pros**: Standardized protocol for IDE integration. Schema-enforced inputs prevent malformed rules.
- **Cons**: No batch/CI mode via MCP. Test data generation requires server-side LLM (MCP sampling not widely supported). Additional server process to manage.

### Agent Skill Only

Purely prompt-based approach with no Go helpers. Relies entirely on the agent's LLM for rule construction and validation.

- **Pros**: Zero infrastructure -- just Markdown files. No code to install or distribute. Works anywhere an agent runs.
- **Cons**: No structural validation -- quality depends entirely on the LLM. No deterministic guarantees. Rules may have syntax errors that only surface at analysis time.

## Future Extensions

- **Language-specific skills**: Specialized rule-writer and test-generator skills for Java, Go, Node.js, C#, and framework-specific migrations (Spring Boot, Quarkus, .NET) with deeper domain knowledge and more accurate pattern extraction
- **Research agents**: LLM-driven discovery of migration paths, breaking changes, and migration guides
- **GitHub mining**: Discover migration patterns from real-world repositories; feed discovered repos into semver-analyzer
- **Knowledge database**: Accumulate migration intelligence over time and use that knowledge to generate rules

## Infrastructure Needed

- **Kantra**
- **CI**: GitHub Actions for running evals
- **Agent runtime**: Any AgentSkills.io-compatible runtime (Claude Code, Cursor, Goose, etc.) for running the full pipeline
