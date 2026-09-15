---
type: Fixed
pr: 307
---
**The cross-process download lock collided in its own staging.** `AcquireLock`
creates its lock object through the same `fileblob` writer everything else
uses, and that writer stages through a temporary file named from a clock which
does not advance on Windows — so two writers of one lock key picked the same
staging path, and the loser's rename found its source already gone.
`TestStagingAndPartialWritesAcrossBuckets` failed on every run because of it.
The write is now serialised within the process the way `Save` already was, and
the `NotFound` that a losing writer raises across processes is recognised as
contention rather than returned as an error (#241).
