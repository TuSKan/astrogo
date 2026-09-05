---
type: Removed
pr: 171
---
**`fits.Write` no longer exists — it always returned `ErrUnimplemented`.** An
exported function that only ever fails, in a package the README marked Stable,
is a runtime surprise for anyone who type-checks against it; an absent one is a
compile error at the call site, which is the honest signal. Its signature could
not have been implemented as written either — a filename and a flat
`[]float64`, with no dimensions or header. `ErrUnimplemented` goes with it, and
the README now labels `fits` read-only, pointing at #127 for the real writer.
Nothing in the module called it (#135).
