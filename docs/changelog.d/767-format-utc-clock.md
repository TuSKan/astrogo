---
type: Fixed
pr: 767
---
**`time.Time.Format` and `FormatJulian` read the UTC clock whatever scale holds the instant.** `Format` did only for years 0–9999 and `FormatJulian` never did, so a TT instant in 585 BC printed 5 h 06 min after itself held as UTC, and in 2026 `FormatJulian` put it 69 s after `Format`. `MST` now reads "UTC" on every path.
