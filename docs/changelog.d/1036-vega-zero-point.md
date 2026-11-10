---
type: Fixed
pr: 1036
---
**`magnitude.Passband.Validate` refuses a Vega zero point that is negative, NaN or infinite**, with the new `ErrPassbandZeroPoint`. Zero still means "unknown". A NaN or infinite zero point used to give a NaN or +Inf Vega magnitude with no error.
