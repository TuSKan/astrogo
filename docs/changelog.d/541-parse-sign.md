---
type: Fixed
pr: 541
---
**`angle.ParseDMS` and `ParseHMS` no longer return the opposite sign for a typeset minus.**
A U+2212 minus, as journals and SIMBAD print negative declinations, was skipped like a separator, so a southern declination parsed as northern.
It is now a sign; text the parsers do not recognize, such as a hemisphere letter, is `ErrSeparator`, and a fourth field is `ErrTooManyFields`.
