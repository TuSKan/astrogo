---
type: Changed
pr: 1271
---
**`remote.SetOffline` refuses only endpoints that reach the network.** An endpoint pointed with `remote.SetURL` at a `file://` path, a `mem://` filesystem or a server on the loopback interface stays reachable offline, so a test can serve its fixtures with offline mode on. An unknown or disabled endpoint is refused for that reason, not as `remote.ErrOffline`.
