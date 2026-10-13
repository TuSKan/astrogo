---
type: Changed
pr: 723
---
**The GAMBONS validation tests read dust from the local SFD map** instead of asking IRSA for each sightline two seconds apart, which used to take longer than the 15-minute timeout allowed. Band medians move by at most 0.001 mag, and the run sends no requests to IRSA.
