# changelog.d

One file per changelog entry, assembled into `CHANGELOG.md` at release time.

## Why

`CHANGELOG.md` was the only file five of eight pull requests conflicted on in
a single batch of parallel work — never the code, always the changelog. Every
branch appends to the same `[Unreleased]` section, so every branch after the
first has to resolve a conflict by hand.

Resolving one textually is not harmless. Merge markers say nothing about which
heading a bullet belongs under, so an entry inherits whichever heading happens
to precede it in the merged text: during the v0.16.0 batch a `### Added` entry
silently became a `### Fixed` one, and the diff looked like whitespace. One
file per entry cannot collide, and cannot lose its own section.

## Adding an entry

Create `changelog.d/<PR-number>-<short-slug>.md`:

```markdown
---
type: Fixed
pr: 61
---
**`Time.ToGo` reinterpreted any scale as UTC.** One instant produced different
`time.Time` values depending on which scale the caller held — measured, 69.184 s
apart. It now converts to UTC first.
```

`type` is one of the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
sections this project already uses: `Added`, `Changed`, `Changed — BREAKING`,
`Deprecated`, `Removed`, `Fixed`, `Security`.

Keep the body to the 1–3 lines `CLAUDE.md` asks for. Forensic detail — root
cause, before/after numbers, live-test confirmation — belongs in the pull
request description or the code's own doc comment. The changelog is an index,
not the record.

## Releasing

```bash
go test ./internal/changelog/ -run TestAssembleRelease -update -release-version 0.17.0
```

That folds every entry here into a new `CHANGELOG.md` section, in the order
above, extends the link-reference chain, and deletes the consumed files. Review
the result before tagging.

The `-update` gate follows this repository's existing convention for generated
artefacts — the Horizons corpus and the accuracy table work the same way — so
nothing rewrites a checked-in file as a side effect of running the tests.

Without `-update`, `go test ./internal/changelog/` checks that every file here
parses and names a valid type. A malformed entry fails ordinary CI rather than
surfacing at release time.
