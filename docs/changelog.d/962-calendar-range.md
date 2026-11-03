---
type: Fixed
pr: 962
---
**Converting a UTC instant before −4900 March 1 to TT no longer panics, and every finite Julian Date has a calendar date.** SOFA's calendar conversion stops at JD −68569.5 and JD 10⁹, and beyond it `Calendar` read 0-0-0 and `DecimalYear` panicked. Dates outside that range now come from the same proleptic Gregorian calendar, carried through its 400-year cycle, and dates inside it are unchanged to the bit.
