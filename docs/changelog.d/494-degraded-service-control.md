---
type: Changed
pr: 494
---
**Network tests no longer fail on a degraded VizieR**, which answers every
query with a 400: a 4xx now skips only when the service also rejects a
control query that cannot be wrong, so a genuinely bad query still fails
(#492).
