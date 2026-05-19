# Story 4.5.3: BMAD Test Fixtures

Status: done

## Story

As a developer,
I want comprehensive test coverage for BMAD detection,
so that the detector is reliable and maintainable.

## Acceptance Criteria

1. Given test fixtures in `test/fixtures/`, then includes BMAD v6 project structures for all major detection scenarios
2. Given vibe-dash's own `.bmad/` folder, then use as real-world dogfooding test case
3. Given unit tests, then covers: detector registration, folder detection, config parsing, stage detection from sprint-status, fallback to artifact detection, and edge cases (missing files, malformed YAML)
4. Given integration test, then verifies end-to-end: add project with BMAD structure, confirm methodology detected as "bmad", confirm stage displayed correctly
5. Given test accuracy measurement, then BMAD fixtures contribute to overall detection accuracy (currently 95% threshold on Speckit, extending to include BMAD)

## Tasks / Subtasks

- [x] Task 1: Create BMAD test fixtures in `test/fixtures/` (AC: #1, #2)
  - [x] Create `bmad-v6-complete/` - full .bmad structure with sprint-status.yaml
  - [x] Create `bmad-v6-minimal/` - just .bmad/bmm/config.yaml
  - [x] Create `bmad-v6-no-config/` - .bmad folder but no config.yaml
  - [x] Create `bmad-v6-mid-sprint/` - with sprint-status.yaml showing epic in-progress
  - [x] Create `bmad-v6-all-done/` - with all epics marked done
  - [x] Create `bmad-v6-artifacts-only/` - has PRD/architecture but no sprint-status
  - [x] Create `bmad-v4-not-supported/` - .bmad-core folder (should not detect)

- [x] Task 2: Create dogfooding test using vibe-dash's own .bmad (AC: #2)
  - [x] Add integration test that detects vibe-dash's own .bmad folder
  - [x] Verify detected method is "bmad"
  - [x] Verify stage matches expected based on current sprint-status.yaml

- [x] Task 3: Extend detector_test.go with fixture-based tests (AC: #3)
  - [x] Add table-driven tests using test fixtures
  - [x] Test all fixture scenarios against expected outcomes
  - [x] Verify CanDetect, Detect, stage, confidence, reasoning for each fixture

- [x] Task 4: Add BMAD to accuracy measurement (AC: #5)
  - [x] Update test/fixtures/README.md to include BMAD fixtures
  - [x] Extend TestDetectionAccuracy to include BMAD fixtures
  - [x] Verify 95% accuracy threshold maintained

- [x] Task 5: Create integration test for full flow (AC: #4)
  - [x] Test `vibe add` on BMAD fixture directories
  - [x] Verify project appears with correct method and stage
  - [x] Test stage updates when sprint-status.yaml changes

## Dev Notes

### Architecture Compliance

- **Test fixture location**: `test/fixtures/bmad-*` (same level as speckit fixtures)
- **Fixture naming convention**: `bmad-{scenario}` or `bmad-v6-{stage}`
- **Test file locations**: `internal/adapters/detectors/bmad/detector_test.go` (extend existing)
- **Integration tests**: Use `//go:build integration` tag for Tasks 2 and 5

### Previous Story Learnings (from 4.5.1 and 4.5.2)

**From Story 4.5.1:**
- Version extracted via regex from file header comment: `# Version: 6.0.0-alpha.13`
- CanDetect is O(1) - only checks if `.bmad/` folder exists
- Detect handles missing config.yaml gracefully with ConfidenceLikely
- Context cancellation pattern: check `ctx.Done()` before AND after every I/O operation

**From Story 4.5.2:**
- Stage detection from sprint-status.yaml uses `development_status` section
- Epic key pattern: `^epic-\d+(-\d+)?$`
- Story key pattern: `^\d+-\d+-`
- Fallback to artifact detection when sprint-status.yaml missing
- Stage mapping: All epics backlog = Specify, Epic in-progress = Plan/Implement, All done = Implement
- Comprehensive test coverage in `stage_parser_test.go` (839 lines, 30+ tests) - extend these patterns

### Fixture Directory Helper Pattern

Use the same pattern as Speckit detector tests:

<!-- stripped fenced code block: go -->

### BMAD Test Fixture Catalog

| Fixture | Expected Stage | Confidence | shouldDetect | Method | Files to Create |
|---------|----------------|------------|--------------|--------|-----------------|
| bmad-v6-complete | Implement | Certain | true | bmad | .bmad/bmm/config.yaml, docs/sprint-artifacts/sprint-status.yaml, docs/epics.md |
| bmad-v6-minimal | Unknown | Likely | true | bmad | .bmad/bmm/config.yaml only |
| bmad-v6-no-config | Unknown | Likely | true | bmad | .bmad/bmm/ (empty) |
| bmad-v6-mid-sprint | Implement | Certain | true | bmad | .bmad/bmm/config.yaml, docs/sprint-artifacts/sprint-status.yaml (epic in-progress) |
| bmad-v6-all-done | Implement | Certain | true | bmad | .bmad/bmm/config.yaml, docs/sprint-artifacts/sprint-status.yaml (all done) |
| bmad-v6-artifacts-only | Implement | Likely | true | bmad | .bmad/bmm/config.yaml, docs/epics.md (no sprint-status) |
| bmad-v4-not-supported | - | - | false | - | .bmad-core/config.yaml only |

### Test Fixture Content Templates

**config.yaml (use for all bmad-v6-* fixtures):**
<!-- stripped fenced code block: yaml -->

**sprint-status.yaml for bmad-v6-complete:**
<!-- stripped fenced code block: yaml -->

**sprint-status.yaml for bmad-v6-mid-sprint:**
<!-- stripped fenced code block: yaml -->

**sprint-status.yaml for bmad-v6-all-done:**
<!-- stripped fenced code block: yaml -->

### Test Helper Functions

Extend existing helpers in `detector_test.go`:

<!-- stripped fenced code block: go -->

### Dogfooding Test Implementation

<!-- stripped fenced code block: go -->

### Extending TestDetectionAccuracy

Add BMAD fixtures to the existing Speckit accuracy test pattern in `detector_test.go`:

<!-- stripped fenced code block: go -->

### Cross-Detector Validation

Ensure neither detector claims the other's fixtures:

<!-- stripped fenced code block: go -->

### Critical Constraints

1. **Fixture location**: Create in `test/fixtures/` alongside Speckit fixtures
2. **Naming convention**: Follow `bmad-{scenario}` pattern
3. **Test file location**: `internal/adapters/detectors/bmad/detector_test.go` (extend existing)
4. **Integration test tags**: Use `//go:build integration` for dogfooding and full flow tests
5. **No duplicate test logic**: Reuse table-driven test patterns from Story 4.5.1/4.5.2
6. **Use fixturesDir() helper**: Same pattern as Speckit tests for path resolution

### DO / DON'T Quick Reference

| DO | DON'T |
|----|-------|
| Create physical fixture directories in test/fixtures/ | Create fixtures only in temp dirs during tests |
| Use `//go:build integration` for slow tests | Run integration tests in normal `go test` |
| Follow pattern from `stage_parser_test.go` for new tests | Create entirely new test patterns |
| Include `generated:` and `project:` fields in sprint-status.yaml | Use minimal YAML that may not parse correctly |
| Use `fixturesDir()` helper for path resolution | Hardcode fixture paths |
| Verify cross-detector exclusion | Assume detectors won't conflict |

### References

- [Source: test/fixtures/README.md] - Existing Speckit fixture documentation
- [Source: internal/adapters/detectors/bmad/detector.go] - BMAD detector to test
- [Source: internal/adapters/detectors/bmad/stage_parser.go] - Stage detection logic
- [Source: internal/adapters/detectors/bmad/detector_test.go] - Existing tests to extend
- [Source: internal/adapters/detectors/bmad/stage_parser_test.go] - Comprehensive stage tests (follow patterns)
- [Source: internal/adapters/detectors/speckit/detector_test.go] - Speckit test patterns including fixturesDir() and TestDetectionAccuracy
- [Source: docs/project-context.md#Testing-Rules] - Testing standards

## Dev Agent Record

### Context Reference

<!-- Path(s) to story context XML will be added here by context workflow -->

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

### Completion Notes List

1. Created 7 BMAD test fixtures in test/fixtures/ covering all detection scenarios
2. Implemented dogfooding integration test using vibe-dash's own .bmad folder
3. Extended detector_test.go with comprehensive fixture-based tests including edge cases
4. Added TestBMADDetectionAccuracy test (100% accuracy on 7 fixtures)
5. Created integration tests for full flow including stage updates
6. Updated test/fixtures/README.md with BMAD fixture documentation
7. Note: bmad-v6-artifacts-only returns ConfidenceCertain (not Likely) because artifact detection returns Likely (not Uncertain) and the detector only downgrades for Uncertain

### Code Review Fixes Applied

1. [H1] Added .gitkeep to bmad-v6-no-config fixture to ensure empty directory is tracked by git
2. [H2] Added bmad-v6-no-config to TestIntegration_FullFlow_DetectBMADFixtures test table
3. [H4] Added fixture existence validation to TestCrossDetectorExclusion
4. [M1] Removed redundant TestBMADDetector_CanDetect_Fixtures test (duplicates TestBMADDetector_FixtureBased)
5. [M2] Added assertion for Phase 1 stage in TestIntegration_StageUpdatesWhenSprintStatusChanges

### File List

- test/fixtures/bmad-v6-complete/.bmad/bmm/config.yaml (new)
- test/fixtures/bmad-v6-complete/docs/sprint-artifacts/sprint-status.yaml (new)
- test/fixtures/bmad-v6-complete/docs/epics.md (new)
- test/fixtures/bmad-v6-minimal/.bmad/bmm/config.yaml (new)
- test/fixtures/bmad-v6-no-config/.bmad/bmm/.gitkeep (new)
- test/fixtures/bmad-v6-mid-sprint/.bmad/bmm/config.yaml (new)
- test/fixtures/bmad-v6-mid-sprint/docs/sprint-artifacts/sprint-status.yaml (new)
- test/fixtures/bmad-v6-all-done/.bmad/bmm/config.yaml (new)
- test/fixtures/bmad-v6-all-done/docs/sprint-artifacts/sprint-status.yaml (new)
- test/fixtures/bmad-v6-artifacts-only/.bmad/bmm/config.yaml (new)
- test/fixtures/bmad-v6-artifacts-only/docs/epics.md (new)
- test/fixtures/bmad-v4-not-supported/.bmad-core/config.yaml (new)
- test/fixtures/README.md (modified - added BMAD fixtures section)
- internal/adapters/detectors/bmad/detector_test.go (modified - added fixture-based tests, accuracy test, cross-detector exclusion, removed redundant test, added fixture validation)
- internal/adapters/detectors/bmad/detector_integration_test.go (modified - added full flow integration tests, added bmad-v6-no-config, added Phase 1 assertion)
