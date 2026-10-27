---
type: Changed
pr: 865
---
**An apparent place on `ephemeris.Default()` evaluates the analytical series
once instead of twice.** The light-time solve carries the Earth or the Sun back
from its state at t while that stays within 1 µas, and evaluates it beyond.
