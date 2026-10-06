---
type: Fixed
pr: 516
---
**The EOP-unavailable warning's remedy now names `remote/eop`**, without which
`remote.EnableDownloads` changes nothing, and docs that still described the
gocloud storage layer — `s3://` and `sftp://` cache examples, a `no_tmp_dir`
parameter the `file://` backend ignores, binary sizes measured against gocloud —
describe the current one.
