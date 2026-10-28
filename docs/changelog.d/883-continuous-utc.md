---
type: Changed
pr: 883
---
**Leap seconds end on 2027-05-20** (CGPM 2026 Resolution 5), not by 2035: the zero-EOP warning states up to
0.9 s of UT1 before that date and whatever |UT1−UTC| has drifted to after it, within the new 3600 s limit,
and the `time` docs and README say the same.
