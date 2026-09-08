---
type: Changed — BREAKING
pr: 220
---
`fits.WCS`'s getters lose their `Get` prefix (`GetCRVAL` is now `CRVAL`), and the
five setters with a length invariant return `error` — a short CRPIX was a panic
reachable from `PixelToWorld` and a long one a silent disagreement about axis
count, both accepted without a word. New `NAxis` reports the invariant (#178).
