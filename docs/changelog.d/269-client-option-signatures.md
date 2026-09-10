---
type: Changed — BREAKING
pr: 269
---
**Twelve constructors gained a variadic option**, so `remote.Client` can reach
them. Ordinary calls are unaffected — `simbad.New()` still compiles — but the
functions' *types* changed, so assigning one to a `func() *Provider` variable or
passing it as a function value no longer compiles. Add the parameter to the
variable's type. Affected: `New` in `catalog/{simbad,sbdb,mast,jpl,norad,vizier,fink,openngc}`,
`fink.NewWithVersion`, `gaia.New`, `mpcorb.Open`, `cams.AOD550`,
`spk.CacheDownload`, `spk.CacheAPI` and `lsk.Cache` (#269).
