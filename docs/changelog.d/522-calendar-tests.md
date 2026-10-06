---
type: Fixed
pr: 522
---
**`time.FromGoDuration` no longer claims to be exact in every case.** A
`unit.Duration` is float64 seconds, which keeps the nanosecond up to about 48.5
days and rounds by up to 4 ns at a year. `Weekday`, `JulianCalendar`, `AddDate`
and `FromGoDuration` are now tested against dates of record.
