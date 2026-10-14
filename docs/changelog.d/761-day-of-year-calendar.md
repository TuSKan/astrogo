---
type: Fixed
pr: 761
---
**`time.Time.DayOfYear` counts within the year `Year` reports.** It measured from Jan 1 midnight in the display location while `Year` ignores it, so 01:00 UTC on 1 January shown in São Paulo came out 0.917, and 23:00 UTC on 31 December shown in Tokyo 366.33 in a 365-day year.
