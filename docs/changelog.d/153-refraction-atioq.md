---
type: Fixed
pr: 153
---
**`coord.Context.GeocentricToObserved` returned altitudes of thousands of
degrees near the horizon.** Its refraction branch wrote out
`Refa·tan(z) + Refb·tan³(z)` with no clamp, so the series diverged just below
the horizon (+7028° at −0.076°) and cancelled to zero just above it (0.000°
where the stellar path applied 0.16°); it also omitted the Newton-Raphson
correction. It now reproduces SOFA's `Atioq` exactly, so the two pipelines
agree to milliarcseconds.
