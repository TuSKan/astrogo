---
type: Fixed
pr: 984
---
**`raster`, `dust` and `Time.Weekday` refuse a NaN instead of turning it into an index, so their answers no longer differ by platform.** `SampleBilinear` reported a NaN location as "no data" on amd64 and returned NaN with no error on arm64, and now calls it outside coverage. `dust.Map.IntensityAt` and `Fetch` could read the galactic center's cell for a NaN direction, and now refuse it with `ErrNoCoverage`. `Time.Weekday` named a day for a non-finite time, and now returns `NoWeekday`.
