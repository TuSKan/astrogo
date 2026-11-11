---
type: Fixed
pr: 1085
---
**Three defects in `starlight`'s Gaia star-map build.** A multi-band `starlight.BuildFromGaia` resuming from its cache
left every band but the first at zero; a NaN or infinite flux in an archive answer went into the map; and
`starlight.Fetch` discarded every batch it had fetched when a later one failed.
