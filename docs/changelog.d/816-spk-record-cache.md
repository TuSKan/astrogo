---
type: Changed
pr: 816
---
**A JPL kernel's sky at one instant is 47× faster from disk, and allocates
nothing** (355 → 7.6 µs, 188 → 0 allocations). An `spk.Reader` now keeps each
segment's directory and last evaluated record, bit-identically; a kernel on
disk, in `mem://` or in memory now costs the same.
