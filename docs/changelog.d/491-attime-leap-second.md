---
type: Fixed
pr: 491
---
**`coord.Context.AtTime` was 13″ off across a leap second**: it reused the
base Context's DUT1, which jumps by a second there. It now takes DUT1 for the
instant it derives, as `NewContext` does (#489).
