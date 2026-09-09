---
type: Added
pr: 234
---
`internal/leakcheck` fails a package's tests when a goroutine outlives them,
naming each survivor and where it started. Installed in the two packages that
fan out — `internal/parallel` and `catalog` — both of which measure clean,
including against the live services (#234).
