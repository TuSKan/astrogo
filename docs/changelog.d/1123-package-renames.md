---
type: Changed — BREAKING
pr: 1123
---
**Packages that shared a name are renamed**: `catalog/fits` is now `catalog/fitscat`, `catalog/jpl` is
`catalog/horizons`, `fits/plan` is `fits/fitsplan` and `skybrightness/plan` is `skybrightness/skyplan`, so no two
packages share a name and none needs an alias beside another. Change the import paths; nothing else moves.
