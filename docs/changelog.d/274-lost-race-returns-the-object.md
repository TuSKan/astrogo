---
type: Fixed
pr: 274
---
**A cache fetch that lost a cross-process race failed instead of returning the object.**
The download lock is exclusive within a process and only mostly so across them, so two
`go test` binaries fetching one kernel could both reach the download; the loser died on
`Access is denied` renaming its staging file while the winner wrote a complete kernel.
`GetFile` now re-runs its freshness check before reporting a fetch failure, so the caller
gets what they asked for when someone else has just produced it (#241).
