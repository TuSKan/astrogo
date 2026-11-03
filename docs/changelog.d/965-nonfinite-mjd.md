---
type: Fixed
pr: 965
---
**A non-finite time no longer panics the Earth-orientation lookup.** `iers.Table.EOP` refused nothing when the MJD was NaN — a NaN passes the range check on both sides — and indexed one past the last record, taking `DeltaT`, `UT1`, `EOP` and every coordinate path that reads Earth orientation down with it for a `Time` whose Julian Date is NaN or ±∞. The table now answers `ErrOutOfRange` for a non-finite MJD, and the skybrightness pivot interpolation, built on the same shape, reports a non-finite wavelength as not from the table instead of walking off its last pivot.
