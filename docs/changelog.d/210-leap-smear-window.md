---
type: Added
pr: 210
---
`time.Time.LeapSmearWindow` reports whether an epoch falls within a day of a
leap second and names the step. An NTP-disciplined host may be deliberately
wrong by up to 0.5 s for up to 24 hours around one — 3.8 km of ISS ground track
— and no library can detect it, so this says where the question arises rather
than answering it (#146).
