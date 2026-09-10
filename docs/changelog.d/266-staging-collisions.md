---
type: Fixed
pr: 266
---
**Concurrent cache writes of one object name collided on Windows.** `fileblob`
stages every write through a temp file named from a clock that does not advance
there — 2000 consecutive `UnixNano` reads returned one distinct value — and puts
it in `os.TempDir`, so writers renamed each other's staging file away, 59 times
in 320. Cache buckets now stage inside themselves, which also avoids re-copying
a multi-gigabyte kernel across volumes, and `file.Save` serialises writers of
one key (#241).
