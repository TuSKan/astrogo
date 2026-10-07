---
type: Changed — BREAKING
pr: 577
---
**`atmosphere.StandardRefraction` and `kepler.PlutoElements` are functions.** As exported vars, one assignment anywhere changed them for every caller in the process; call them as `atmosphere.StandardRefraction()` and `kepler.PlutoElements()`. The `unit` and `dim` vars stay, documented read-only rather than immutable, and a module-wide guard requires a stated reason for any exported var that is not a sentinel error.
