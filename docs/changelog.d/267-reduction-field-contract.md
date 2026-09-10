---
type: Added
pr: 267
---
`coord.Reduction`'s fields now say what they are and how they relate. Two are
positions and two are directions, and since #262 put diurnal aberration on the
direction path, rotating `Topocentric` by hand no longer reproduces `Geometric`
— they differ by up to 0.32″, which is now measured by a test rather than left
to be discovered (#267).
