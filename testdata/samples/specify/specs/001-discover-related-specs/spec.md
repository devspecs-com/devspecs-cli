# Feature Specification: Probabilistic related specs (CLI)

**Feature Branch**: `001-discover-related-specs`  
**Created**: 2026-05-10  
**Status**: Draft  
**Input**: User description: Summarize user-visible behavior from the probabilistic related specs plan: `ds related`, `ds workon`, `ds mine`, evidence buckets, Git hook triggers; omit storage internals from the stakeholder-facing wording.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See likely related specs from a path (Priority: P1)

A developer points the assistant at an ordinary project file path and learns which specification-style artifacts are most likely related, with short, human-readable reasons they can trust as hints (not proof).

**Why this priority**: This is the core outcome: faster navigation from code to the right spec without manual repo archaeology.

**Independent Test**: Run the “related” command against a known file in a sample repository and verify ranked results, bucket labels, and readable evidence lines (text and structured output).

**Acceptance Scenarios**:

1. **Given** the tool has indexed the repository and the user names a repository file path, **When** they run the related command with default options, **Then** they see artifacts grouped as high and medium likelihood, each with one or more evidence lines explaining why.
2. **Given** the same context, **When** they enable the flag that includes low-likelihood matches, **Then** low-likelihood artifacts appear in the output alongside higher buckets.
3. **Given** the same context, **When** they request machine-readable output, **Then** they receive a stable structured response that includes bucket or score information suitable for tooling, without requiring them to parse free-form text.

---

### User Story 2 - Mine the repo for link evidence (Priority: P1)

A developer runs a command to analyze recent or broader history (per their chosen scope) and record probabilistic links between artifacts and files for later queries, with optional quiet operation suitable for automation.

**Why this priority**: Without fresh evidence, “related” results go stale; mining is how the product keeps suggestions current.

**Independent Test**: In a temporary repository with scripted commits containing spec and code touch patterns, run the mine command with recent scope and verify link evidence is produced and counts or summaries are visible (including quiet mode).

**Acceptance Scenarios**:

1. **Given** a repository with recent changes relevant to specs, **When** the user runs the mine command with default target (current repository) and a “recent” scope, **Then** the tool completes and reports that mining ran (or stays silent in quiet mode), and subsequent related queries reflect new or updated evidence where applicable.
2. **Given** the same repository, **When** the user chooses a broader history scope, **Then** the tool still completes within reasonable interactive use (guarded by conservative limits on work for very large histories).
3. **Given** automation or hooks, **When** the user runs the mine command in quiet mode, **Then** ordinary success does not spam the console with progress noise.

---

### User Story 3 - Tie active work to an artifact (Priority: P2)

A developer declares which specification artifact they are “working on” so that later mining can treat their current branch and touched files as extra context for likely relationships.

**Why this priority**: Active session context reduces false negatives when the important signal is “what I am doing right now” rather than only historical commits.

**Independent Test**: Start a session for a known artifact, confirm the tool reports it as active, clear the session, and confirm no active session remains.

**Acceptance Scenarios**:

1. **Given** an indexed artifact identifier the user intends to focus on, **When** they start a work session with that identifier, **Then** the tool records an open session scoped to that repository context (branch and current commit identity as applicable).
2. **Given** an open session, **When** the user asks for session status without arguments, **Then** they see which artifact is active or a clear message that none is.
3. **Given** an open session, **When** the user clears the session, **Then** the tool closes the open session and subsequent status shows no active session.

---

### User Story 4 - Git hooks keep the index and links fresh (Priority: P3)

After one-time setup, common Git operations automatically refresh the local index and run targeted mining so related results stay useful without the user remembering commands.

**Why this priority**: Habit formation and low friction; reduces stale suggestions for teams that opt in.

**Independent Test**: Install hooks, perform representative Git events, and observe the configured commands run in quiet modes without duplicating hook blocks on repeat setup.

**Acceptance Scenarios**:

1. **Given** hooks are installed, **When** the user creates a commit, **Then** the repository is scanned for changes (when needed) and recent mining runs, both in quiet modes suitable for hook output.
2. **Given** hooks are installed, **When** the user checks out a different revision, **Then** a quiet scan runs to refresh index state for the new tree.
3. **Given** hooks are installed, **When** the user completes a merge or history rewrite that updates the working tree, **Then** a quiet scan runs and recent mining runs afterward.
4. **Given** hooks are already installed, **When** the user runs the install command again, **Then** hook entries are not duplicated (idempotent setup).

### Edge Cases

- **No active work session**: Related and mine behavior must not require a session; work-session evidence only applies when the user has started one.
- **Paths outside the repo or ambiguous paths**: The tool must resolve or reject paths with a clear, actionable message.
- **Very large history**: Broader mining scopes must remain bounded so a single invocation does not appear hung; partial or capped work is acceptable if communicated or documented in product help.
- **False positives**: Copy and UX should present results as probabilistic hints (“likely related”), not authoritative blame or ownership.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The CLI MUST provide a “related” command that accepts a file path (repository-relative or absolute within the repo), normalizes it consistently, and returns candidate artifacts ranked by likelihood.
- **FR-002**: For each candidate artifact in default “related” output, the CLI MUST show human-readable evidence lines explaining contributing signals (for example same change set as code, mentions in artifact text or tasks, shared area heuristics, or active-work context when applicable).
- **FR-003**: The CLI MUST map each artifact’s aggregated likelihood to exactly one of three user-visible buckets — high, medium, or low — using these numeric thresholds on an internal score in the range zero to one inclusive: high when score is at least 0.75, else medium when at least 0.45, else low when at least 0.20; scores below 0.20 MUST be treated as non-matches for bucketing.
- **FR-004**: When combining multiple evidence contributions for the same artifact and file, the CLI’s scoring rule MUST add contributions and cap the total at 1.0 before bucket assignment.
- **FR-005**: Default “related” output MUST include only high and medium buckets; the user MUST be able to include low bucket matches via an explicit flag.
- **FR-006**: The CLI MUST provide a “mine” command operating on the current repository by default, with user-selectable scope at least between “recent” history and “all” history (broader scope subject to safety limits in FR-012).
- **FR-007**: The “mine” command MUST persist probabilistic artifact–file link evidence for later use by the “related” command and MUST support a machine-readable summary mode (for example bucket counts) suitable for scripts.
- **FR-008**: The “mine” command MUST support a quiet mode that suppresses non-essential console output on success.
- **FR-009**: The CLI MUST provide a “workon” command that opens a work session for a given artifact identifier, shows current session status when invoked without starting a new session, and closes the active session when the user requests clear.
- **FR-010**: Starting a new work session for the same repository work context MUST end any prior open session for that context so at most one active session applies per scope defined by the product (repository root, worktree, and branch).
- **FR-011**: When a work session is active and mining runs, evidence attributable to the user’s current branch and files touched in the mined change set MUST be eligible for recording as work-session context (so “related” can reflect what the user is actively doing).
- **FR-012**: Mining over “all” history MUST use conservative limits (maximum commits and/or files processed) to protect interactive use; behavior when truncating MUST be predictable (for example complete successfully with bounded work rather than failing by default).
- **FR-013**: The product MUST offer optional Git hook installation that runs, in quiet modes: after a commit — scan when needed and then recent mining; after checkout — scan only; after merge or history rewrite — scan and then recent mining.
- **FR-014**: Re-running hook installation MUST not duplicate hook blocks (idempotent install).

### Key Entities *(include if feature involves data)*

- **Artifact**: A specification or similar tracked item the product indexes; users refer to it by stable identifier for work sessions.
- **File path (normalized)**: A repository-relative path with consistent separator rules used as the join key between mining output and “related” queries.
- **Evidence record**: A single contributing reason tying an artifact to a file (signal type, human-readable detail, confidence weight, first and last time seen).
- **Aggregated link**: The per-artifact, per-file combined score derived from evidence records under FR-004, used for ranking and buckets.
- **Work session**: An open or closed interval associating a repository work context with an artifact the user is focusing on, driving optional work-session evidence during mining.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In controlled fixture repositories, at least 90% of scripted “related” checks return the expected top artifact within the high or medium bucket when the plan’s signal scenarios are present (same change set, explicit reference, mention, directory heuristic, or active work session).
- **SC-002**: On mid-size sample repositories (on the order of thousands of files), a “recent” mining invocation completes in under two minutes on a typical developer machine, or the product enforces a cap and completes without manual cancellation (per FR-012).
- **SC-003**: After hook installation, 100% of duplicate install runs in tests produce a single set of hook hook lines (no duplicated command blocks).
- **SC-004**: In user interviews or internal dogfooding, at least four of five developers report that “related” output helps them choose the correct spec faster than browsing alone (qualitative questionnaire after one week of use).

## Assumptions

- Users work in Git-backed repositories; non-Git roots are out of scope.
- “Artifact” identifiers and discovery behavior match whatever the product already indexes today; this feature adds probabilistic linking and session context, not a new document format.
- Default branch naming for comparing history follows conventional remote defaults (`main` or `master` or equivalent origin symbolic ref) unless the user has configured otherwise in existing product behavior.
- First release may treat repository root as the worktree path; linked worktrees are an optional refinement if parity is not already universal.
- Machine-readable field names remain stable minor-version to minor-version for scripting; any breaking structured output changes will be signaled through the product’s normal release communications.
