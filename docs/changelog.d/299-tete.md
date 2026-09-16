---
type: Added
pr: 299
---
`coord.TETE` is the apparent place referred to the true equator and true
equinox of date — what almanacs and most telescope control systems mean by
"apparent RA and Dec". `coord.Context.CIRSToTETE` converts to it from
`coord.CIRS`, which measures right ascension from the Celestial Intermediate
Origin instead. The two are apart by the equation of the
origins: **20.3 arcminutes in 2026**, growing by 46 arcseconds a year (#126).
