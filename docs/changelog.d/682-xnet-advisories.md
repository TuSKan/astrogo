---
type: Security
pr: 682
---
**`golang.org/x/net` v0.60.0, for five HTTP/2 advisories published 2026-10-08**: GO-2026-6617, -6612, -6611, -6610 and -6603 (CVE-2026-97032, -78663, -78669, -78660, -78659), covering an HPACK encoder race, flow-control and window-update abuse, malformed framing headers and Trailer memory exhaustion. `govulncheck` found them reachable from astrogo's HTTP clients.
