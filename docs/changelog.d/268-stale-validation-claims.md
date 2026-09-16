---
type: Fixed
pr: 268
---
`docs/VALIDATION.md` said a smeared clock was untracked when
`time.Time.LeapSmearWindow` already handled it, and carried an SGP4 limitation
whose shape had changed. A document that tells readers a safety signal does not
exist while shipping it is worse than one that says nothing (#268).
