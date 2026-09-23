---
type: Changed
pr: 372
---
**`testutil.SkipOnUpstreamFailure` now answers the whole question**, consulting
`Unreachable` for the cases it did not cover — a DNS failure, a refused or
unroutable dial. A test no longer has to call both to find out whether a failure
was somebody else's, which is friction that helped make a skip-on-any-error the
convenient thing to write. A caller's own canceled context still does not skip.
