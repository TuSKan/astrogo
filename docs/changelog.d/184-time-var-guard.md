---
type: Added
pr: 184
---
A guard holds `time`'s exported package-level vars to a documented inventory of eight,
each recording why Go leaves no alternative — six error sentinels, `LocationUTC` (which
wraps the standard library's own mutable `time.UTC`) and `J2000` (#113).
