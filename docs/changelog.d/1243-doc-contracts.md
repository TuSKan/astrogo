---
type: Changed
pr: 1243
---
**Every package's doc comments state the contract** rather than what the code used to do, and `remote`, `catalog` and `fits` say which values are safe to share across goroutines. Three package docs that described the code wrongly are corrected: `time`'s, on what registers the EOP loader; `catalog`'s, on how results are cached; and `ephemeris`'s, on the binary size the kernel import adds.
