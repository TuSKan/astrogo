---
type: Fixed
pr: 652
---
**`catalog.Resolver` dropped FINK's H, G1, G2, spin and oblateness**, even from a result FINK alone found, so `plan.FromCatalog` had no H to build an asteroid from. The physical-parameter cluster now takes SBDB's V-band photometry first and FINK's whole sHG1G2 fit where SBDB has none, never mixing the two.
