---
type: Added
pr: 275
---
**The FITS conventions real files use, in both directions.** HIERARCH keywords and CONTINUE
long strings (previously mangled and truncated on *read*, not merely unwritten); unsigned
images through BZERO; DATASUM and CHECKSUM on every HDU, satisfying the sum-to-all-ones test
cfitsio and astropy apply; TNULL and NaN so a missing table value stays missing; vector
columns, which used to decode as nulls and discard every value; and ASCII tables, whose
reader consumed the payload without decoding it (#127).
