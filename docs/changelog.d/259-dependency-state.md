---
type: Changed
pr: 259
---
`resty.dev/v3` moves to `rc.4`, and `go.mod` now records why `gocloud.dev`
cannot leave its pseudo-version: `gocloud-ext`'s httpblob driver implements
`blob/driver.DeleteOptions`, which was added after v0.46.0 (#259).
