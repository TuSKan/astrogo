---
type: Removed
pr: 1273
---
The `remote/eop` package was removed, and its blank import is no longer needed. The loader API it used was removed with it: `epoch.RegisterEOPLoader`, `epoch.EOPLoader`, `epoch.FSEOPLoader`, `epoch.EOPData`, `epoch.ErrNoEOPLoader` and `epoch.ErrNoEOPData`. A bulletin from elsewhere is `remote.SetURL` for `remote.IERSFinals2000A`, or `epoch.ParseFinals2000A` with `epoch.RegisterModel`.
