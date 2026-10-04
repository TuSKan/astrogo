---
type: Fixed
pr: 449
---
**`time.Time.FormatJulian` rounds to the nearest second**, and
`DateJulianCal` keeps its day and time of day as separate Julian-date parts:
half of all whole-second Julian-calendar times printed a second early, and
`Format` did the same for years outside 0–9999 (#439).
