---
type: Fixed
pr: 563
---
**Satellite standard magnitudes now reproduce themselves.** `magnitude.SatelliteApparent` measured both conventions' phase correction from 90° and converted Molczan magnitudes to McCants ones, so a McCants magnitude came out up to 1.24 mag too bright and a Molczan one 1.45 mag too bright at their own reference geometry. Each convention is now applied at its own reference phase, full phase for McCants and 90° for Molczan, and a Molczan-based prediction stays about 0.7 mag fainter, as the convention's definition says.
