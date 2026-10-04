---
type: Fixed
pr: 458
---
**A partial download is no longer thrown away when its ETag sidecar cannot be
read** — a virus scanner holding the fresh file open on Windows read as no
ETag — and the read is retried before it is given up on (#452).
