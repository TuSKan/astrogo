---
type: Added
pr: 269
---
**`remote.Client` makes I/O policy a value instead of process state.** Two
components in one binary can now hold different ones — an offline request path
beside a prefetcher that downloads — which `SetOffline` made impossible. Every
package-level function still operates on `remote.Default`, and every consuming
constructor gained an option (`jpl.WithClient`, `resolve.WithClient`,
`cams.WithClient`, `api.WithRemote`) that leaves ordinary calls untouched — see
the breaking note for the one case that does change (#114).
