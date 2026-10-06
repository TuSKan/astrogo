---
type: Removed
pr: 536
---
**The last `Deprecated` symbols are gone.** The mutable maps
`plan.KnownSites` and `plan.MeteorShowers` (since removed; use `KnownSiteNames`/`NewKnownSite`, `MeteorShowerNames`/`NewMeteorShower`),
`plan.TwilightThresholds` and `jpl.BodyIDToNAIF` (since removed; use `TwilightThreshold`, `NAIFFor`/`NAIFBodies`),
and the sentinel `plan.ErrNotCoordObject`, which nothing returned (since removed).
