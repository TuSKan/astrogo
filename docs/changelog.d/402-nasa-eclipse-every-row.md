---
type: Fixed
pr: 402
---
**The NASA eclipse validation compares every catalog row.** It had dropped 78
eclipses whose type codes its parser did not list; with them, solar is
1433/1433 and lunar 1451/1452, and the ΔT cross-validation now asserts a bound
instead of logging (#398). The lunar miss and 173 over-reported eclipses are
#401.
