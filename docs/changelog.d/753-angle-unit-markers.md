---
type: Fixed
pr: 753
---
**`angle.ParseDMS` and `ParseHMS` read unit markers as what they say.** They were separators only, so fields were placed by count: `30'` parsed as 30°, `45"` as 45°, and `12h30m00s` given to `ParseDMS` as 12.5° rather than 187.5°. A minute or second marker now names its field, an hour or degree marker sets the unit, and a marker out of order is the new `ErrMarker`.
