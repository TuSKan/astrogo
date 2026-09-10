---
type: Fixed
pr: 268
---
`docs/VALIDATION.md` said SGP4 had **no runtime signal** separating the validated
regime from the 3440 km one, and that a smeared clock was untracked. Both had
been resolved — by `satellite.Satellite.Verified` (#182) and
`time.Time.LeapSmearWindow` (#146) — so the document was telling readers a
safety signal did not exist while shipping it (#268).
