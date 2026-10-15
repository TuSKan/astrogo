---
type: Fixed
pr: 800
---
**A `fits.WCS` can be shared by goroutines transforming through it.** `PixelToWorld` and `WorldToPixel` re-derived the projection from `CTYPE` and wrote it into the `WCS` on every call, a data race when one `WCS` was shared. It is now derived when `CTYPE` is set, so the transforms only read, and a TAN `PixelToWorld` takes about 115 ns instead of 184.
