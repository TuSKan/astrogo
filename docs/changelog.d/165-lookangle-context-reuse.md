---
type: Fixed
pr: 165
---
**`plan.LookAngle` discarded the `coord.Context` it was handed** and had
`coord.Reducer` build a second one, repeating the Apco13 solve the first
already held. Measured: 242 → 95 µs per call, and `SatellitePasses` 318 → 200
ms over a six-hour window, since it paid the solve twice per 30-second sample.
Everything the Reducer computed is already on `Context`, so nothing is rebuilt.
Using the given Context is also what the signature promised — one derived by
`Context.AtTime` used to be silently replaced with a full rebuild (#111).
