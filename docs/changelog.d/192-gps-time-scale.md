---
type: Added
pr: 192
---
**`time.GPST` — GPS system time, the scale a satellite user most often actually holds.**
A receiver timestamp had no way to say what it was; calling it UTC is 18 s wrong today,
which is 138 km of ISS track. GPST is TAI − 19 s exactly, so the conversion is arithmetic
and cannot fail. Galileo shares it; BeiDou and GLONASS do not (#145).
