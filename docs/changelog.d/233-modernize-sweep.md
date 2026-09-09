---
type: Changed
pr: 232
---
Five `sync.WaitGroup.Add`/`Done` pairs become `wg.Go`, removing the class of
bug where the two get out of step, and eleven `sort.Strings`/`Float64s` calls
plus the data-driven `sort.Slice` sites become `slices.Sort`/`SortFunc` —
measured 1.7× faster with no allocation. `Planner.RankObservable` also stops
mis-ordering a ranking that contains a non-finite score.
