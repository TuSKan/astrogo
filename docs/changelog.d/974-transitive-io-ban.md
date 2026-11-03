---
type: Fixed
pr: 974
---
**`skybrightness`'s rule that evaluation performs no I/O is checked across the whole import closure and against the real components.** The structural check looked at direct imports only, on the stale premise that `coord`, `ephemeris` and `time` reach `remote`. It now bans `remote`, the network packages and the dataset formats at any depth. The offline test, which ran two constant stubs, now runs the six real components offline first and requires identical radiance online.
