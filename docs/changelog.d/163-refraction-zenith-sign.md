---
type: Fixed
pr: 163
---
**`RefractionRigorous` and `RefractionApproximate` returned negative refraction
near the zenith** — −0.114″ and −0.080″, crossing zero at 89.89° and 89.92°,
because the term that stabilises each fit near the horizon carries its tangent
argument past 90° at the top. Both now return zero above the crossing, which is
the physical limit and costs at most 0.001″ against a fit quoting 6″. The
known-values test also stops taking `math.Abs` before comparing, which had made
its bracket blind to the sign of every row, not just the zenith one (#162).
