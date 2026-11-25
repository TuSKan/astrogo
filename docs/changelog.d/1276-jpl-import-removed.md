---
type: Removed
pr: 1276
---
The blank import of `ephemeris/jpl` is no longer needed, since `ephemeris.NewProvider` reaches the kernel provider directly, and the registration behind it was removed: `core.KernelBackend`, `core.KernelRequest`, `core.RegisterKernelBackend`, `core.KernelProvider`, `core.HasKernelBackend` and `core.ErrNoKernelBackend` no longer exist.
