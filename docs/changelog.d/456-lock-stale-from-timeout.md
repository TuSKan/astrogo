---
type: Fixed
pr: 456
---
**A download lock left by a crashed process goes stale after twice the
download timeout** rather than always 30 minutes: a minute for the IERS
bulletin, whose lazy load, with consent, held every EOP lookup behind the
wait (#445).
