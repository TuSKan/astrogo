---
type: Removed
pr: 319
---
`sgp4.ErrDeepSpace` — the scaffold that refused a deep-space element set while
SDP4 was being written. Deep space now propagates, so nothing raises it. It was
added and removed inside the same unreleased cycle and never appeared in a
release, so no released code can be matching on it; a caller building against
`main` between [#317] and this change should delete the branch. [#310]
