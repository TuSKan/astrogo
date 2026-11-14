---
type: Fixed
pr: 1145
---
`plan.GreatestElongations` and `plan.Appulses` no longer report the end of the search window as an event. From some start times the sweep's last two samples matched by rounding, and the window's last seconds came back as a greatest elongation or an appulse, one of them 167° wide.
