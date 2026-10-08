---
type: Fixed
pr: 637
---
**`remote.SetOffline` made `remote.GetFile` refuse even an object already in the cache**, so the README's air-gapped recipe failed: with DE440s pre-seeded, `eph.NewProvider` worked online and returned "offline mode enabled" offline. In offline mode a cached object is now served, Mutable or not, and only a miss fails with `ErrOffline`.
