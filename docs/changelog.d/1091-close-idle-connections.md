---
type: Fixed
pr: 1091
---
**`remote.Client.Close` and `SetAPIOptions` release the idle connections they claim to**: resty's own `Close` never
reaches the transport, so every idle connection outlived both calls by the transport's 90-second idle timeout.
