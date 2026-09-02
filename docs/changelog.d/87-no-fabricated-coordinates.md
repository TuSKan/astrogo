---
type: Changed — BREAKING
pr: 87
---
**`plan.FromCatalog` returns `(Observable, error)`** and refuses a fixed target
with no position instead of placing it at RA 0, Dec 0 — a real point in Pisces
that rises, sets and schedules without complaint. Returns `plan.ErrNoCoordinates`.
