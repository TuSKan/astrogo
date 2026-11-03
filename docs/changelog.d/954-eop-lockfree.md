---
type: Changed
pr: 954
---
**`Time.EOP` reads Earth orientation without a lock.** A covered lookup is 66–89% faster on one core. It now scales across cores, where sixteen goroutines used to look up fewer epochs a second than one. A program with no EOP loader no longer serializes every lookup on a global mutex. Results are bit-identical.
