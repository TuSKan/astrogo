---
type: Added
pr: 82
---
**CI now requires a changelog fragment naming the pull request.** Both
`CONTRIBUTING.md` and `docs/PULL_REQUESTS.md` asked for one and nothing checked,
so #80 and #81 merged without and would have been missing from their release.
Label a pull request `no-changelog` when it genuinely warrants no entry.
