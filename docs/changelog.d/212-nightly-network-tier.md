---
type: Changed
pr: 212
---
The `network` tier now runs nightly rather than weekly, in its own job; the
`validation` and `integration` tiers stay weekly. Only the network tier notices
a SIMBAD schema change or a Horizons format change — `go vet -tags` cannot, the
code still compiles — and a week is a long time to be wrong about that. Still
off the PR path (#123).
