---
type: Removed
pr: 319
---
`sgp4.ErrDeepSpace` no longer exists. It was the scaffold that refused a
deep-space element set while SDP4 was being written, and deep space now
propagates, so nothing raises it. Added and removed inside the same unreleased
cycle, so no released code can be matching on it; a caller building against
`main` between [#317] and this change should delete the branch. [#310]
