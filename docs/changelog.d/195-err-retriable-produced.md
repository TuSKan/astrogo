---
type: Fixed
pr: 195
---
**`remote.ErrRetriable` was produced by nothing.** A 503 that survived every retry and a
404 were the same error, so a caller could not tell "the service was busy and we gave up"
from "you asked for something that is not there". `APIClient.Get` now wraps the final
`*remote.HTTPError` with it whenever the retry policy would have retried that status.
