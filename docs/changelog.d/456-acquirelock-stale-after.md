---
type: Changed — BREAKING
pr: 456
---
**`remote/file.AcquireLock` takes a `staleAfter` duration**, the age past which
a lock is taken for a crashed holder's; zero keeps the 30-minute default.
`remote/file` is internal to `remote` — docsguard rejects importing it from
anywhere else — so only `remote` itself calls it (#445).
