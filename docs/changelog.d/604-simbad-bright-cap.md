---
type: Fixed
pr: 604
---
**`plan.VisibleTonight` only ever considered SIMBAD's 100 brightest objects**, down to V 2.46, whatever its magnitude limit, because `simbad`'s `SearchBright` took `TOP 100` for a request with no limit. It now returns every object, up to SIMBAD's 50,000-row default. Past that it reports `simbad.ErrBrightTruncated` rather than a short list. At Paranal, at limit 4.5, that is 632 objects where it was 86.
