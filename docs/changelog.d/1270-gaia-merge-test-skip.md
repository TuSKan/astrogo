---
type: Fixed
pr: 1270
---
**`TestResolverMergesGaiaForMovingStarsLive` failed when Gaia stopped answering between two stars**, since the Resolver reports nothing of a Gaia failure once SIMBAD has answered. It now asks Gaia's control before failing.
