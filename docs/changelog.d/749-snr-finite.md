---
type: Fixed
pr: 749
---
**`optics.Instrument.SNR` and `LimitingSignal` refuse a NaN or infinite signal, background, read noise or dark current** with `ErrExposure`. They answered NaN, 0 or +Inf with no error, and a NaN signal-to-noise ratio fails every detection threshold silently.
