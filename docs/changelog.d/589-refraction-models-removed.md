---
type: Removed
pr: 589
---
**`atmosphere.RefractionApproximate` no longer exists, and neither does `atmosphere.RefractionRigorous`.** They were the same two formulas, Saemundsson's and Bennett's, and "Rigorous" integrated nothing. `atmosphere.RefractionBennett` replaces both: Bennett's formula as refitted to the Nautical Almanac's tables, which it reproduces within 0.12′, inverted for the forward direction so the round trip is exact.
