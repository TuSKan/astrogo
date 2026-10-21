---
type: Fixed
pr: 819
---
**`jpl.Provider` chained a kernel's segments whatever their reference frame**
and labeled the result ICRF, so an ECLIPJ2000 kernel came out 23.4° off with
no error. A kernel holding a segment in any frame but J2000 is now refused
whole, with `jpl.ErrUnsupportedFrame`; every kernel astrogo fetches is J2000.
