---
type: Changed — BREAKING
pr: 531
---
**`magnitude.GaiaGToJohnsonV`, `GaiaGToJohnsonB`, `GaiaGToJohnsonR` and
`GaiaGToCousinsI` now return `(mag, ok)`**, with `ok` false outside the BP−RP
interval each relation was fitted over. `catalog/gaia` reports no V outside it
instead of an extrapolation — an L dwarf at BP−RP = 6 came back at V = 16.6 for
G = 12. Callers that used the single value take the first result and check `ok`.
