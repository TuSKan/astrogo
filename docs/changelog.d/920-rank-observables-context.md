---
type: Changed
pr: 920
---
**`plan.RankObservables` builds one `coord.Context` for the instant instead of one per target**: 54–62% faster and 86% fewer bytes, with
every score unchanged to the last bit. `ConstraintCtx.CheckCtx` now documents that it must only read the Context it is given, since the
goroutines ranking the other targets read the same one.
