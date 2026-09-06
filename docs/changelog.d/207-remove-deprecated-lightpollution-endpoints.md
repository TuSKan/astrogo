---
type: Removed
pr: 207
---
`remote.WorldAtlas` no longer exists, and neither does `remote.LightPollution`;
both were deprecated in 0.15.0 and are past the two minor releases the policy
requires. Nothing read either: one was a non-commercially-licensed model output
that cannot validate `skybrightness` and must not be served as its answer, the
other read satellite radiance as sky brightness, which is on that module's
prohibited list (#119).
