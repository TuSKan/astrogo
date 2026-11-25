---
type: Removed
pr: 1267
---
The download-consent API was removed: `remote.EnableDownloads`, `remote.DisableDownloads` and `remote.DownloadsEnabled`, their `remote.Client.EnableDownloads`, `remote.Client.DisableDownloads` and `remote.Client.DownloadsEnabled` methods, and the `remote.Endpoint.DownloadsOK` and `remote.Endpoint.MaxDownloadSize` fields. A policy passed to `remote.SetPolicy` expresses never, a size cap and only these endpoints.
