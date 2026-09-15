---
type: Fixed
pr: 306
---
**Proper motions from SIMBAD and Gaia were applied short by a factor of
cos(dec).** Both catalogues publish μα\* — the on-sky rate — and every consumer
in `coord` handed it to SOFA, which wants dRA/dt. A star propagated twenty years
moved 20·cos δ arcseconds instead of 20: 30% short at δ = 45° and 83% short at
δ = 80°, putting Kapteyn's Star 49″ from where it is. `coord` now converts at
the one boundary where SOFA is called, so a catalogue row travels to a position
unchanged at every layer between (#281).
