---
type: Changed
pr: 1101
---
**Every package that starts a goroutine runs its tests under the leak check, and a test keeps it so**: `skybrightness`,
whose `Model.SkyMap` evaluates directions on an errgroup, ran none. The list of such packages was prose in
`internal/leakcheck`, and it had gone stale.
