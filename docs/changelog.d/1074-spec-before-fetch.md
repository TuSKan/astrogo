---
type: Fixed
pr: 1074
---
**`dataset.Inputs` checks the whole `Spec` before fetching anything.** A set but invalid grid was silently replaced
with the default, a star shape of the wrong length was returned unchecked, and most other mistakes were refused only
after one to four fetches. `airglow.Spec` refuses a negative, NaN or infinite `Scale`, which it had read as 1 or let
make the spectrum infinite, and non-finite ranges, steps and fluxes.
