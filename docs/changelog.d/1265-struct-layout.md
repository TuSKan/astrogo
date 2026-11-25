---
type: Changed — BREAKING
pr: 1265
---
**Struct fields have been reordered to reduce alignment padding.** Use keyed
literals for `remote.Endpoint` and `skybrightness.Contribution`. Code that
depends on declaration order should account for the new layout.
