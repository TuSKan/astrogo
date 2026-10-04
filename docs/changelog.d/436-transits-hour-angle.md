---
type: Fixed
pr: 436
---
**`EventSolver` finds a meridian transit wherever the hour angle rises through
zero**, not only beside a sampled altitude maximum: transits at high latitude,
in a window's first step, or with a fine step are no longer dropped, and the
Sun's are no longer 1.3 s early from aberration applied twice (#417).
