---
type: Fixed
pr: 854
---
**The AD 33 eclipse example computed the sunset and moonrise of Julian 5 April
for an eclipse on the 3rd**, building a historical date with the Gregorian
`time.Date`. Historical dates in the examples and `docs/JESUS.md` now use
`time.DateJulianCal`, the doc quotes what the code and NASA's canon give, and
docsguard refuses a pre-1583 `time.Date` in code a user copies.
