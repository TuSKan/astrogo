---
type: Fixed
pr: 62
---
**A bare number could load a different object entirely.** `NewProvider(ctx, core.SmallBody, "1")` returned comet 1000036, not 1 Ceres, because a bare number resolves against Horizons' major-body and comet indices before the numbered-asteroid record — and every mechanical check passed, so nothing said so. A numbered asteroid must now arrive as `core.SmallBodyID(n)` or the load fails with `jpl.ErrWrongSmallBody`.
