---
type: Removed
pr: 594
---
**`magnitude.ExtinctionV`, `ExtinctionB`, `ExtinctionU`, `ExtinctionR` and `ExtinctionI` were since removed, and `magnitude.ExtinctionAtAltitude` no longer exists.** The coefficients had no source, and the altitude scaling thinned ozone and aerosol as if they were air molecules. Use `atmosphere.Atmosphere.Extinction` for a site's own air at the band's wavelength.
