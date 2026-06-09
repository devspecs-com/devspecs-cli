# BMAD Fixture

This fixture intentionally keeps only a tiny synthetic `_bmad-output` tree.

It exists to test BMAD output layout detection without shipping the full
installed BMAD method bundle in the public CLI repo.

## Included

- `_bmad-output/planning-artifacts/prd.md`
- `_bmad-output/planning-artifacts/architecture.md`

Both files are synthetic and exist only as parser/discovery fixtures.

## Excluded

The installed `.agents/skills/**` and `_bmad/**` method bundle is deliberately
not included. It was too large for the public fixture surface, did not have an
obvious local license/provenance file in the copied fixture, and was not needed
by the public BMAD detection tests.
