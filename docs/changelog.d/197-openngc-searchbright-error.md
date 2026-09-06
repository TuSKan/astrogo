---
type: Fixed
pr: 197
---
**`openngc.Provider.SearchBright` reported an unreachable catalog as an empty
sky.** It was the one query that never consulted the recorded load failure, so a
denied download or a dead endpoint came back as "no objects brighter than that"
rather than as an error. It now yields the failure through the iterator.
