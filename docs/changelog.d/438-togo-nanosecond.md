---
type: Fixed
pr: 438
---
**`time.Time.ToGo` keeps the nanosecond**: it summed both Julian-date parts
into one float64 count of seconds since 1970 and lost up to 119 ns, so
`FromGo(t).ToGo()` was not the identity for most instants (#420).
