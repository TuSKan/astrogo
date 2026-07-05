// Package lightpollution resolves a site's artificial night-sky brightness from
// the lightpollutionmap.info QueryRaster service (the World Atlas 2015 layer,
// i.e. the Falchi et al. 2016 atlas) and converts it to a skybrightness floor.
//
// The QueryRaster API requires a free API key (https://www.lightpollutionmap.info,
// 500 requests/day). Supply it via [WithAPIKey] or the LIGHTPOLLUTIONMAP_KEY
// environment variable. No data is bundled and nothing is fetched unless you
// call a client method.
//
// # Brightness → magnitude
//
// Luminance and surface brightness are related by the standard photometric
// relation L[cd/m²] = 1.08×10⁵·10^(−0.4·m), anchored to the natural zenith
// background 1.71168465×10⁻⁴ cd/m² ≡ 22.0 V mag/arcsec² (Falchi et al. 2016). The
// World Atlas layer reports ARTIFICIAL brightness (mcd/m²); the natural
// background is added in linear luminance before converting to a total
// V mag/arcsec².
//
// # Accuracy
//
// The atlas is a 2015 VIIRS-calibrated model of the artificial component only;
// it does not include the Moon, zodiacal light, airglow, or transient
// conditions. Combine the returned floor with the time-dependent
// skybrightness components for an observing-time estimate.
//
// References:
//   - Falchi et al. 2016, "The new world atlas of artificial night sky
//     brightness", Sci. Adv. 2, e1600377.
//   - lightpollutionmap.info QueryRaster service (Jurij Stare).
package lightpollution
