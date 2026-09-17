---
type: Fixed
pr: 354
---
**`coord`'s au-per-Julian-year constant was wrong in its ninth digit.** It
shipped as the literal `4.740470446`, a decimal repeated widely in the
literature, while its own doc comment described it as "the au divided by the
Julian year" — which is `4.740470463533`, from 149597870700 m over
365.25 × 86400 s, both exact by definition since IAU 2012 Resolution B2. The
constant is now computed from `constants.IAU.AstronomicalUnit` rather than
written down, so it cannot drift from the au again. The error was 3.7e-09
relative and nothing observable moved — the Sun's rotational velocity in
`SolarVelocityFromSgrA` shifts by 9.2e-07 km/s — but a constant whose
documentation describes a different number than it holds is a trap for the next
reader. `coord/spacevelocity_test.go` held the same wrong decimal, which is why
no test caught it; it now asserts the value independently.
