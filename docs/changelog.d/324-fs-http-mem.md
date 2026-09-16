---
type: Added
pr: 324
---
`remote/file`'s `io/fs` core gains `http`/`https` and `mem://` backends. The
HTTP one is read-only by design — no astrogo source accepts a write — and keeps
one body open across sequential reads while each `ReadAt` takes its own range.
`mem://` exists for the write path that `fstest.MapFS` cannot cover, keyed by
URL host so two opens can share a store or deliberately not.
