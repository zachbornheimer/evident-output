# Exported API surface

The exported surface is [`testdata/api_golden.txt`](../../testdata/api_golden.txt).
`mise run api-contract` fails when the live package differs from it, when a
name in `testdata/api_required.txt` is missing, or when a name in the retired
table (`internal/retired`) comes back. Read the golden file for what exists;
this page does not repeat it, so it cannot go stale.

Removed names and their replacements are in [`CHANGELOG.md`](../../CHANGELOG.md)
and the migration guides under [`docs/migration/`](../migration/).

## Honesty rule

No public field or parameter may be ignored (PHIL-007). A value the API builds
but nothing consumes is a defect, and so is a Config zero value that silently
does nothing.
