// Package lpmap resolves a site's artificial night-sky brightness from the
// lightpollutionmap.info QueryRaster service (the World Atlas 2015 layer,
// i.e. the Falchi et al. 2016 atlas) and converts it to a skybrightness floor.
//
// lpmap is a live-API sibling of [github.com/TuSKan/astrogo/skybrightness/atlas]:
// both resolve the same kind of artificial-brightness data for the same
// purpose (a [skybrightness.Floor] input), but atlas decodes a
// caller-supplied (or, via atlas.EnsureWorldAtlas, automatically
// downloaded) offline file, while lpmap queries the service live. For most
// callers atlas is the better default — see
// [github.com/TuSKan/astrogo/skybrightness/atlas.Resolver] for a resolver
// that tries atlas first and falls back to lpmap only if configured with a key.
//
// # Getting an API key
//
// There is no self-serve signup. The QueryRaster key is issued manually,
// one at a time, by emailing the service's owner, Jurij Stare
// (starej@t-2.net) — see https://www.lightpollutionmap.info/help.html. The
// free tier is 500 requests/day. Supply the key via [WithAPIKey] or the
// LIGHTPOLLUTIONMAP_KEY environment variable. No data is bundled and
// nothing is fetched unless you call a client method.
//
// # Layers
//
// Two "ql" families exist, reporting different physical quantities:
// "wa_2015" (World Atlas 2015, mcd/m² artificial brightness — the
// default) and "viirs_<year>" (raw VIIRS-DNB radiance, nW·cm⁻²·sr⁻¹).
// [Client] dispatches on the family to interpret the returned number
// correctly (see [WithLayer]); a name in neither family returns
// [ErrUnknownLayer].
//
// The service publishes no machine-readable list of valid "ql" values —
// its help page documents the map UI and the bulk GeoTIFF downloads, not
// the QueryRaster parameters — so this client deliberately does NOT
// hardcode a year whitelist it cannot verify or keep current. It
// validates the family (which determines the UNIT, and getting that wrong
// yields a plausible-looking wrong brightness rather than an error) and
// lets the server be authoritative on which years it actually carries.
// For reference, the site's own bulk downloads run 2012 through 2025 and
// gain a year annually.
//
// The 500/day figure is a usage-pattern quota, not a burst rate — the service
// documents no per-second limit, so there is nothing meaningful for this
// client to throttle at request time. It does retry transient failures and
// 429/5xx responses with exponential backoff (bounded, see maxRetries); a
// sustained 429 past that budget surfaces as [ErrBadResponse], and callers
// approaching the daily quota are responsible for their own call cadence.
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
package lpmap
