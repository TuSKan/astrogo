---
type: Fixed
pr: 785
---
**Opening a cached JPL kernel no longer fails on Windows while another process records its checksum.** The kernel's SHA-256 sidecar was read once, and a sharing violation from another process renaming its own sidecar into place failed the open of a good kernel. It is now retried briefly, and a refused write accepts another process's identical record.
