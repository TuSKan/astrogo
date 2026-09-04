---
type: Fixed
pr: 148
---
**The kernel-driven conversion returned TT, not TDB.** Its formula omitted the TDB−TT
periodic term (~1.7 ms amplitude — 1.7 m of lunar motion, 85 m for Mars). The
conversion is now delegated to `time.Time.TDB`, which owns leap seconds for the
library; the function's `*lsk.Reader` parameter is retained for compatibility
and is no longer read.
