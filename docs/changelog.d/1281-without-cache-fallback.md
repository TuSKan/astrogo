---
type: Added
pr: 1281
---
**`remote.WithoutCacheFallback`** has `remote.GetFile` return a Mutable endpoint's cached copy only when its source confirms it, failing offline, under a refusing policy or with the source not answering, for a caller that reads the cache itself.
