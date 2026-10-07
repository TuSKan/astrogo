---
type: Fixed
pr: 585
---
**The FINK integration tests no longer fail on FINK's bad moments.** A 200 holding nothing usable is retried, and then checked against a control query: an empty control skips the test as a degraded service, and a live one fails it as wrong data. Each distinct query is fetched once per run, two requests where there were five.
