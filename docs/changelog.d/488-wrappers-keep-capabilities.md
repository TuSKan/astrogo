---
type: Fixed
pr: 488
---
**An endpoint or data dir reached through `?prefix=` or `?key=` could never
download**: both wrappers hid the backend's write, lock and cancel interfaces.
They now carry them, and `OpenFS` refuses a backend whose capabilities a
wrapper cannot keep (#487).
