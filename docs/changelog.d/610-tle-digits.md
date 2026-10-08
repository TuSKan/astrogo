---
type: Fixed
pr: 610
---
**`catalog/norad`'s `GP.ToTLE` lost the last digit of eccentricity and B\* to floating-point scaling**, and wrote a zero exponent as `-0` where CelesTrak writes `+0`. It now writes these fields from the GP value's decimal digits, and reproduces CelesTrak's published TLE for 11,345 of 11,357 satellites on line 1 and all of them on line 2.
