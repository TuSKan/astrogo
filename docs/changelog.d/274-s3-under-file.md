---
type: Changed — BREAKING
pr: 274
---
**`remote/s3` moved to `remote/file/s3`.** S3 is a file backend, so the opt-in blank
import now sits where the file code does: `import _ "github.com/TuSKan/astrogo/remote/file/s3"`.
Still four lines and zero exported symbols, and still the module's only importer of the
AWS SDK.
