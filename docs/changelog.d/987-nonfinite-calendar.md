---
type: Fixed
pr: 987
---
**A non-finite `Time` no longer gets a definite calendar date, clock or day of the year.** `DayOfYear` returned −1721059, `JulianCalendar` −4712-01-01, and `ToGo`, `Format` and a located `String` the year 292277019622 on amd64. Now `DayOfYear` is NaN, `JulianCalendar` is zeros as `Calendar` is, `ToGo` is the zero `time.Time`, and the formatters print `JD NaN`.
