---
type: Fixed
pr: 844
---
**Two Jerusalem examples placed the city about 470 km off**, with latitude and
longitude swapped in `coord.NewGeodetic`. The examples, docs and `Example`
functions now build sites with `coord.NewEarthLocation`, and every online
example imports `remote/eop` for real Earth orientation. `internal/docsguard`
holds both.
