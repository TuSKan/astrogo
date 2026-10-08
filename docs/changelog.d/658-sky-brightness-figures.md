---
type: Fixed
pr: 658
---
**The documented sky-brightness figures were not what the tests measure.** CLAUDE.md, the README, the design document and VALIDATION.md quoted a near-full Moon at 18.9 mag/arcsec², the single-scattering figure from before Winkler's multiple-scattering factor; `TestScatteredMoonlightFullMoonSkyBrightness` measures 18.6. CLAUDE.md's full-sky Paranal figure, 21.5, predates integrated starlight; the scene now comes out at 21.3. The VALIDATION row now cites the test that produces its figure.
