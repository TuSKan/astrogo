---
type: Security
pr: 164
---
**SIMBAD and VizieR are now addressed over HTTPS.** Both were registered as
cleartext `http://`, shipping catalogue queries and the positions they return
where anything on the path could read or rewrite them. Verified against the
live services first: both answer HTTPS byte-identically to HTTP. `www.cvrl.org`
stays HTTP because it runs no TLS listener at all — port 443 refuses the
connection — and now says so in a new `Endpoint.InsecureReason`, which
`TestNoUndeclaredCleartextEndpoints` requires of any cleartext URL in the
registry (#107).
