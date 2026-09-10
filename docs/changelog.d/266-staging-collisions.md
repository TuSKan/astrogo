---
type: Fixed
pr: 266
---
**Concurrent cache writes of one file name collided on Windows.** `fileblob`
stages every write in `os.TempDir` under a name built from the key's basename
and a nanosecond clock that does not move there — 2000 consecutive reads
returned one distinct value — so writers renamed the staging file out from
under each other, 31 to 69 times in 320. `file.Save` now holds a lock keyed on
that basename (#241).
