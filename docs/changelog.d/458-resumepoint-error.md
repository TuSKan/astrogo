---
type: Changed — BREAKING
pr: 458
---
**`remote/file.ResumePoint` returns an error** beside the offset, for a partial
whose sidecar or size cannot be read. `remote/file` is internal to `remote` —
docsguard rejects importing it from anywhere else — so only `remote` itself
calls it (#452).
