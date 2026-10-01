# Index Maintenance

## Diagnose Local State

`ds doctor` inspects the active binary, PATH precedence, DevSpecs home, index
schema and writer state, and repository identity. It is offline and read-only:
it does not create local state, migrate or repair the index, or contact the
network.

```bash
ds doctor
ds doctor --redact
ds doctor --json --redact
```

Use `--redact` before sharing a report. If doctor finds a shadowed binary,
correct PATH and restart the shell or IDE. For an older index schema, run the
current CLI normally to migrate forward. For a newer schema, update the CLI
before deciding whether to rebuild the local index. A held writer is an
instantaneous observation; wait for the active operation and retry. For index
growth, inspect `ds prune --dry-run` before choosing `ds prune` or
`ds prune --vacuum`.

An unreadable index is not a reason to delete it blindly. Preserve it while
collecting the redacted report, then use the explicit index recovery commands
below. Recovery is not part of `ds doctor`. Repository identity probing is
bounded at five seconds; a timeout leaves the other diagnostic evidence intact
and reports a warning.

## Back Up And Recover The Index

DevSpecs keeps the SQLite index rebuildable, but recovery is backup-first. A
supported forward migration creates and verifies a rollback snapshot before it
publishes the new schema. The explicit maintenance commands are:

```bash
ds index backup
ds index rebuild --path .
ds index restore <backup-file>
```

`backup` also works when the database schema is newer than the running CLI.
`rebuild` performs the same full cold scan as normal indexing, validates the
replacement, and preserves the displaced index. `restore` publishes the exact
selected snapshot without migrating it and backs up the index it replaces.

To return to an older CLI, restore a snapshot created by that CLI generation as
your final current-CLI index action, then launch the older binary. Running a
newer indexed command again may migrate the snapshot forward. Backups are local
under `~/.devspecs/backups/index/`; manual backups are not automatically
removed.

The backup-first guarantee starts with the release that includes `ds index`.
DevSpecs v1.4.0's compatibility `ds scan --rebuild` deleted the active index
before rescanning. If v1.4.0 is still installed, update first or copy
`~/.devspecs/devspecs.db` while DevSpecs is idle before using that old rebuild
path.
