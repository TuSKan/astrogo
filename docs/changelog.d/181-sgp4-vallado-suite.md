---
type: Added
pr: 181
---
**SGP4 is now verified against Vallado's reference vectors** (AIAA 2006-6753),
checked in and run by the `validation` tier: 588 states over 30 element sets,
reported as a distribution. 22 cases agree to p50 35 m / max 289 m; 8 diverge by
0.6–3440 km, asserted from both sides so neither a regression nor a fix passes unnoticed.
