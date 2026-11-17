---
type: Added
pr: 1216
---
`remote.Client` remembers a host that gave no answer and fails requests to it at once with `remote.ErrUnreachable` for a cooldown, five minutes or `remote.Client.SetUnreachableCooldown`, so one outage costs a program one timeout rather than one per call.
