---
type: Fixed
pr: 842
---
**The universal-variable path (`kepler.FromPerihelion`) lost up to 317 ulps of
position** to cancellation in the Stumpff functions and in g and ġ far from
perihelion. Every regime is now within 4.7 ulps, held to a 256-bit oracle.
