---
type: Added
pr: 1064
---
**`skybrightness/dataset/viirs` has a fuzz target, `FuzzOpen`**, which opens fuzzed annual archives through
`viirs.Open` and on through sampling and emitter building, and holds that chain to never panicking.
