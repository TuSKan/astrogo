---
type: Changed
pr: 189
---
**The `time` package doc and `docs/VALIDATION.md` now record that an epoch read from a
smeared host clock is not UTC.** Around a leap second, NTP providers spread the step over
as much as 24 hours — each differently, none announcing which — so `NowUTC` can be off by
0.5 s with nothing to detect it: 0.3″ of lunar motion, 3.8 km of ISS track (#146).
