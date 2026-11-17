---
type: Changed — BREAKING
pr: 1221
---
**`coord.NewContext` and `coord.NewReducer` return an error**: a nil site is `coord.ErrNilGeodetic`, a time that is not finite `coord.ErrNotFinite`, and air that fails `Validate` its `atmosphere.ErrRefraction`. They panicked on the first and gave NaN places for the others. `Context.ICRSToAltAz`, `ICRSToHourAngle` and `AltAzToICRS` refuse input that is not finite with `coord.ErrNotFinite`.
