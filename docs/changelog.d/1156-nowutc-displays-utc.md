---
type: Fixed
pr: 1156
---
`time.NowUTC` displays its instant in UTC. It kept the host's local zone, so `NowUTC().Format("15:04 UTC")` printed the local clock labeled UTC.
