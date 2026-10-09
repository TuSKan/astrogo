---
type: Fixed
pr: 701
---
**`Time.DecimalYear` ran backward within a month**: it added the day's fraction as if it were the month's and dropped the day of the month, so 2026-03-01 23:59 read later in the year than 2026-03-31. It now gives the fraction of the year elapsed, and ΔT before 1960 is no longer read up to half a month off (#697).
