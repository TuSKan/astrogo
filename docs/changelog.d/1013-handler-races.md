---
type: Fixed
pr: 1013
---
**22 test servers record what they saw without a data race.** Each wrote a variable the test read once its call returned, and a retried request could leave an earlier attempt's handler still writing it: the race detector caught this in `catalog/vizier`. A new internal `testutil.Recorded` holds such values under a mutex, and counters are atomic.
