---
type: Added
pr: 1276
---
**`jpl.WithKernel`** loads a further planetary kernel after the primary one, as `ephemeris.WithKernel` does; each is listed under its key in `jpl.Provider.LoadedKernels`, and a kernel that fails to load closes those opened before it.
