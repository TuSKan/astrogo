---
type: Fixed
pr: 629
---
**`catalog.Resolver` never applied proper motion when cross-matching by position**, so a star faster than about 0.125″/yr never matched its own Gaia row: HD 189733, tau Cet and Barnard's star took nothing from Gaia. Positions now move by the Target's own kinematics, and `resolve.ConeRequest` gains `Epoch` so Gaia and VizieR search around the center moved to their own catalog's epoch.
