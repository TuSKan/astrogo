---
type: Changed — BREAKING
pr: 1244
---
**A latitude and a longitude are `angle.Latitude` and `angle.Longitude`, and every signature takes the latitude first, so a swapped pair does not compile.** `coord.NewGeodetic`, `coord.MustGeodetic`, `coord.NewEcliptic`, `coord.NewGalactic`, `coord.NewObserversLocation`, `coord.HEALPix.PixelOf`, `coord.SkyOffset.ToICRS` and the dust, raster, starlight and VIIRS samplers take them; `coord.HEALPix.Center`, `coord.SkyOffset.FromICRS` and `raster.Grid.LatLon` return them, latitude first. Each of `coord.NewEarthLocation`, `plan.NewSiteEarthLocation` and `raster.Grid.LonLat` was removed: build a site from degrees with `coord.NewGeodetic(angle.LatDeg(lat), angle.LonDeg(lon), unit.Meters(h))` and `plan.NewSite`.
