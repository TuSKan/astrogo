---
type: Added
pr: 274
---
**GCS, Azure Blob Storage and SFTP join S3 as opt-in bucket backends** —
`remote/file/{gcs,azure,sftp}`, each a blank import with zero exported symbols registering
`gs://`, `azblob://` and `sftp://`. Every connection detail rides in the endpoint URL, so
`remote.SetDataDir("gs://my-bucket")` is the whole configuration. Verified: each pulls only
its own SDK, and a build that opens none links none of them.
