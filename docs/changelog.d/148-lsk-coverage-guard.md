---
type: Added
pr: 148
---
**A guard that makes the LSK prove its own coverage.** Every assignment in the
kernel's data block must be one the parser models, and every constant the
parser claims must be non-zero — so an unrecognised keyword in a future kernel
revision fails a test instead of being silently dropped, which is how the
relativistic constants went unread for a release.
