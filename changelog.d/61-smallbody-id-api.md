---
type: Changed — BREAKING
pr: 61
---
**`Provider.SupportedBodies` reports small bodies as `core.SmallBodyID(n)`**, not the bare number: `SmallBodyID(433)` rather than `core.ID(433)`. A bare `core.ID(433)` still resolves in `State`, so only code that matches against the enumerated list needs updating. Adds `core.SmallBodyID`, `core.SmallBodyBase` and `ID.SmallBodyNumber`.
