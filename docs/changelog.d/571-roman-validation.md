---
type: Added
pr: 571
---
**Constellation lookup is now checked against Roman (1987)'s boundary table.** 199,999 of 200,000 random positions agree, and a test holds the 64 lying within about 22″ of a boundary. astropy's `get_constellation` is not used as the reference, because it carries 18–22″ of annual aberration into the lookup.
