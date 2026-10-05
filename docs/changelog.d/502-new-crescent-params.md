---
type: Removed
pr: 502
---
**`NewCrescentParams` is gone from `plan`**: use `CrescentVisibility`, which
finds the evening's sunset and moonset itself. `NewCrescentParams` gave every
criterion one set of parameters, with a constant lunar semi-diameter and a lag
estimated from the Moon's altitude (#496).
