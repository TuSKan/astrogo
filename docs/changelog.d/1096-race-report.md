---
type: Changed
pr: 1096
---
**A failed race shard names what failed and keeps its `race.json`**: the step reports `go test`'s exit status and any
`jq` failure separately, reads the stream without `--slurp`, and uploads `race.json` when the job fails, after three
failures in one day left nothing in the log.
