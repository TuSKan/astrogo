---
type: Added
pr: 274
---
**`remote.Client` scopes I/O policy to a component instead of a process.** Offline mode,
download consent, endpoint URLs and the cache location are now a value: `remote.NewClient()`,
configure it, hand it to the component that owns it. An HTTP handler can be offline-only
while a background prefetcher downloads, in one binary. The package-level functions operate
on `remote.Default()` — the `http.DefaultClient` analogue — so nothing changes for a program
with a single policy. Closes #114.
