---
type: Changed
pr: 222
---
`examples/` is now its own Go module, so its 32 demo programs no longer appear
in astrogo's package listing — they were 32 of 84 rows. Run one with
`go -C examples run ./<name>`; a `replace ../` keeps them building against the
working tree, and CI compiles and lints them separately (#124).
