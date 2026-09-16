---
type: Fixed
pr: 342
---
**FK4 and FK5 round trips no longer drift away from the catalogue equinox.** A
star with no recorded proper motion still moves in both frames, because each
drifts against the inertial one, and `ICRSToFK4`, `FK5ToFK4` and `ICRSToFK5`
hand that fictitious motion back rather than pretending the star is at rest.
They marked it as a *recorded* motion, which sent the inverse down the
six-element branch — SOFA's `Fk425` for FK4, `Fk52h` for FK5 — and both of those
take no epoch and assume the catalogue equinox, while the position they were
handed is at the caller's epoch. The error was linear in the distance from the
equinox and exactly zero at it, which is why every existing round-trip test
missed it: `ICRS → FK4 → ICRS` lost 0.474″ at B2050 and `ICRS → FK5 → ICRS`
0.096″ at J2100, at 4.7 and 0.96 mas per year respectively. `FK4` and `FK5` now
record whether a motion was measured or supplied by the frame — the same
distinction #278 gave `ICRS` — and dispatch on it. Both round trips close at
every epoch, the FK4 one to the 0.000023″ floor SOFA's own E-term iteration
leaves and the FK5 one exactly; `TestAstrogoMatchesRawSOFAAtEveryEpoch` now pins
astrogo to within a nanoarcsecond of the raw `Fk54z` → `Fk45z` pair, which always
closed and so localised the defect to astrogo's dispatch rather than to SOFA.
Closes [#341].
