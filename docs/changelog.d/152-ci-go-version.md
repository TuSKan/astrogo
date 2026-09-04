---
type: Changed
pr: 152
---
**CI selects Go by minor version instead of reading go.mod.** The `go 1.25.8`
directive — inherited from gocloud-ext, not needed by astrogo (#109) — made
`setup-go` demand that exact patch and download a toolchain before every job,
which hung for 25 minutes on one run. `go-version: '1.25'` resolves to whatever
1.25.x the runner already has.
