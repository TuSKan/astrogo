---
type: Removed
pr: 173
---
**42 `//nolint:gochecknoglobals` directives suppressed a linter `.golangci.yml`
disables** — 29% of every suppression in the tree, each carrying a documented
reason for silencing nothing, which is what made the live suppressions hard to
pick out. All removed. `TestNoNolintForADisabledLinter` now cross-checks every
directive against the config, since a dead one is invisible to golangci-lint
itself: `nolintlint` reports an *unused* directive, but one naming a disabled
linter is simply skipped (#136).
