---
type: Fixed
pr: 62
---
**A Horizons refusal arrived as success.** Asked to generate an SPK for 101955 Bennu, Horizons answers "SPK creation is not available for pre-computed objects in the major body index" — a complete explanation, which `spk.CacheAPI` never parsed. The caller got an empty kernel list and a nil error, then a misleading complaint about designation syntax that was never wrong. The service's own sentence now reaches the caller as `spk.ErrHorizonsRefused`.
