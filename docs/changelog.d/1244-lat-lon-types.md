---
type: Changed — BREAKING
pr: 1244
---
**A latitude and a longitude are `angle.Latitude` and `angle.Longitude`, and every signature takes the latitude first, so a swapped pair does not compile.** `coord.NewGeodetic`, `coord.MustGeodetic`, `coord.NewEcliptic`, `coord.NewGalactic`, `coord.NewObserversLocation`, `coord.HEALPix.PixelOf`, `coord.SkyOffset.ToICRS` and the dust, raster, starlight and VIIRS samplers take them; `coord.HEALPix.Center`, `coord.SkyOffset.FromICRS` and `raster.Grid.LatLon` return them, latitude first. `plan.NewSite` takes the latitude, longitude and height itself, `plan.NewSite("Quinta Calixto", angle.LatDeg(-22.528478), angle.LonDeg(-46.473002), unit.Meters(835.05))`, and returns `coord.NewGeodetic`'s error for a location it refuses. Each of `coord.NewEarthLocation`, `plan.NewSiteEarthLocation`, `plan.ErrNilLocation` and `raster.Grid.LonLat` was removed.
