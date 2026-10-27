---
type: Fixed
pr: 870
---
**The docs compile check never type-checked a call through `eph.`, `plan.` or
`jpl.`, and checked only the README.** It now honors shown imports, knows `eph`,
and compiles every document, which found four broken samples in
`docs/PLANET_PARADE.md`.
