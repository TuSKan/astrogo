---
type: Fixed
pr: 626
---
**`docs/VALIDATION.md` claimed 1e-4 for `atmosphere.Airmass` against "analytical"**, for a test that bounded the horizon value between 35 and 42. It is now held to Pickering's published formula and to pvlib's independent implementation to 1e-12, and its doc says it is the molecular airmass.
