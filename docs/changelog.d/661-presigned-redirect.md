---
type: Fixed
pr: 661
---
**The SFD dust map could not be downloaded**, and the nightly network tier failed on it from late September. Dataverse now redirects to a pre-signed S3 URL that refuses the HEAD `remote`'s file probe made; a 403 there now falls back to a ranged GET. Behind it, `remote.SFDDustMap`'s and `remote.OpenNGC`'s `ApproxSize` were below their real files, so a grant of `ApproxSize` was refused; both are now measured, and a network test holds every listed file to its endpoint's budget.
