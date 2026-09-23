---
type: Fixed
pr: 392
---
**SBDB's bright-object query returns elements at full precision.** It rounded
them to four significant figures, as the single-object lookup rounded to three
before it asked for `full-prec`: C/1937 C1's e = 1.000162271 came back as
1.0002, and its perihelion time to a hundredth of a day.
