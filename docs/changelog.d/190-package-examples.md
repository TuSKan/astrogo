---
type: Added
pr: 190
---
Runnable `Example` functions for `time`, `coord`, `ephemeris`, `atmosphere`, `magnitude`
and `catalog` — the task each package exists for, on pkg.go.dev's front page. All but
`catalog`'s carry an `// Output:` comment, so they are executed and diffed by every
`go test` rather than merely compiled (#141).
