---
type: Fixed
pr: 365
---
**A network test no longer fails when the service's own TLS certificate does.**
An expired certificate, or one signed by an untrusted authority, now skips like
any other upstream outage — it passed the TCP pre-check and matched no network
predicate, so `api.open-elevation.com`'s lapsed renewal made a permanent red
build. A certificate valid for a *different* name stays fatal: that means
astrogo asked for the wrong host, which is the defect these tests exist to
catch.
