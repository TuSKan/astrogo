---
type: Added
pr: 275
---
**`fits.Write` — the package writes FITS as well as reading it.** Images at every BITPIX
(uint8, int16/32/64, float32/64) and binary tables built from an Arrow batch, to any
`io.Writer`. Structural keywords are derived from the data rather than copied from the
stored header, so a filtered table or a replaced image cannot produce a file whose header
describes something it does not contain. A card that will not fit the 80-byte record is an
error rather than a truncation, since an over-long card shifts every card after it (#127).
