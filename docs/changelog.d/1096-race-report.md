---
type: Fixed
pr: 1096
---
**A failed race shard reports its failing tests and keeps its `race.json`**: GitHub runs the step under `bash -e`, so a
failing `go test` ended it before the report ran, and every failed shard printed nothing. Each exit status is now
caught and reported, and `race.json` is uploaded when the job fails.
