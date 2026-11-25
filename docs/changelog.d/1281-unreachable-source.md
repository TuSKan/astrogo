---
type: Fixed
pr: 1281
---
**With no network and offline mode off, a cached catalog or bulletin is served instead of refused**, after asking the source once under its download timeout. A cached OpenNGC catalog failed to resolve a name, and a stale IERS bulletin cost the first lookup 1 min 24 s behind a firewall dropping packets, where it now costs one 21 s timeout; a host that gave no answer is not asked again for another file inside the client's cooldown.
