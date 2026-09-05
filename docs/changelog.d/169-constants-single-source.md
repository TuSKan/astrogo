---
type: Changed
pr: 169
---
**The astronomical unit was written out in four production files and the
light-time constant in two**, all agreeing with `constants` and with each other
— which is the problem, since a future revision in `constants` would leave five
copies silently behind. All now derive from `constants`.
`jpl.KMPerAU` no longer exists: it was exported, letting a downstream caller
pin the stale value, and nothing outside its own file referenced it.
`TestCanonicalConstantsAreNotWrittenOut` scans the module so the literals
cannot come back (#137).
