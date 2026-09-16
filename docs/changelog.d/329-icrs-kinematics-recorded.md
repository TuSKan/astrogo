---
type: Fixed
pr: 329
---
**`ICRSToFK4` no longer invents proper motion for a star declared at rest.**
It chose its conversion route by testing every kinematic field for zero, which
cannot tell "no proper motion recorded" from "measured as zero" — different
claims about a star that convert differently — and answered the first when
asked the second. A star declared at rest in ICRS came back from
`ICRS → FK4 → ICRS` carrying **0.6 to 0.9 mas/yr** it never had, while its
position closed to 19 µas, which is what kept it invisible. `coord.ICRS` now
records whether kinematics were supplied, as `coord.FK4` has always done
through its two constructors, so both routes are chosen rather than inferred
and both round trips close. `ICRSToFK5` gets the same treatment. Closes [#278].
