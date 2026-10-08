---
type: Fixed
pr: 619
---
**`catalog/jpl` resolved common names to the wrong body, or to none**: ISS to Larissa, Moon to the Earth-Moon barycenter, Voyager 1 to the ID `"spacecraft"`, and Halley to nothing. `Search` now ranks Horizons' whole answer by name before capping it, both match tables are read by their columns, a single match's ID is its last parenthetical (or a comet's record number), and a DASTCOM record number is no longer stored as `SPKID`.
