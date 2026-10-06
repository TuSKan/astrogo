---
type: Fixed
pr: 546
---
**`plan.Conjunctions` now finds conjunctions in right ascension of date**, as the almanacs define them.
It compared right ascension along the J2000 equator, which put conjunctions up to 2.5 min off; all four checked against Skyfield on DE440s now agree within a second.
