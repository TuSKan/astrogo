---
type: Fixed
pr: 198
---
**A leap second passed to `time.Date` was aliased silently.** `23:59:60` has no
representation in a two-part JD whose day is 86400 seconds long, so it collapsed
onto the following midnight — one second away, and converted with the wrong ΔAT
(37 rather than 36). It still does; it now says so through `logging` at WARN,
distinguishing a real leap second from a second that never existed. See #144 for
the representation question, which stays open.
