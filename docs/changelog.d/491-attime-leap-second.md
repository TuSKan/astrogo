---
type: Fixed
pr: 491
---
**A `coord.Context` stepped across a leap second was 13″ off**: it reused the
base Context's DUT1, which jumps by a second there. Each instant now takes its
own DUT1, as `NewContext` does (#489), and `coord.Context.SetTime` keeps it.
