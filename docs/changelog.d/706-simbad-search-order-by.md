---
type: Fixed
pr: 706
---
**`simbad.Provider.Search` answered HTTP 400 on every call** since v0.19.0: its query ordered by a qualified column, which SIMBAD's TAP parser rejects. It now orders by `main_id`, and a live test runs `Search`, which none did.
