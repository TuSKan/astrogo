---
type: Fixed
pr: 581
---
**`plan.SublunarPoint` puts the Moon at the zenith.** `coord.SubPoint` took the point whose ellipsoid normal is parallel to a body's direction, which treats the Moon as infinitely far: the Moon sat up to 11.7″ from the zenith there, and the sublunar latitude was 4.3″ from Skyfield's. It now takes the foot of the normal through the body's position, which also gives a satellite's sub-satellite point; the Sun and planets are unchanged.
