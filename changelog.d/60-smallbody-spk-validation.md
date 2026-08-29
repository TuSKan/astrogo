---
type: Added
pr: 60
---
**Small-body SPK evaluation is now validated against Horizons.** The only assertion on a small-body position was `0.1 AU < |r| < 5.0 AU` — a bound spanning two orders of magnitude, guarding the hand-rolled SPK **Type 21** decoder where a defect once corrupted positions. Astrogo agrees with Horizons to **33 mm** across four bodies from 1 to 5 AU and eccentricity 0.08 to 0.89.
