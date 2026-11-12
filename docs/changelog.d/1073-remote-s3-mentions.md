---
type: Fixed
pr: 1073
---
**`dataset.LiveAerosol`'s doc and `remote.CopernicusEODATA`'s registered description no longer tell callers to
import `remote/s3`**, a package that went with gocloud.dev. Both now say the endpoint is unreachable at present,
since no `s3://` backend is registered.
