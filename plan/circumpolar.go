package plan

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
)

// standardRefractionCircumpolar is the same ~34' standard atmospheric
// refraction at the horizon Site.SunRiseSetThreshold/MoonRiseSetThreshold
// already use — kept as its own named constant here (rather than reusing
// theirs) since neither of those methods exposes the bare number.
const standardRefractionCircumpolar = 0.5667 // degrees

// circumpolarConfig holds IsCircumpolar/IsNeverUp's options.
type circumpolarConfig struct {
	threshold    angle.Angle
	hasThreshold bool
	refraction   bool
}

// CircumpolarOption customizes IsCircumpolar/IsNeverUp.
type CircumpolarOption func(*circumpolarConfig)

// WithRefraction includes standard atmospheric refraction at the horizon
// (~34', the same constant Site.SunRiseSetThreshold/MoonRiseSetThreshold
// use) in the altitude threshold. Off by default, matching
// Site.RiseSetThreshold's own documented convention for a generic point
// source — real horizon refraction slightly widens the circumpolar zone
// and narrows the never-rises one, so which way this defaults changes the
// answer right at the boundary. Ignored if WithHorizonAltitude is also given.
func WithRefraction() CircumpolarOption {
	return func(c *circumpolarConfig) { c.refraction = true }
}

// WithHorizonAltitude overrides the horizon reference with a caller-supplied
// minimum altitude instead of the site's true, elevation-corrected horizon
// — e.g. a fixed local obstruction ("never clears my treeline"). Takes
// precedence over WithRefraction, since a caller-chosen minimum altitude
// already IS their effective horizon. (Not named WithMinAltitude: that name
// is already VisibleTonight's, for an unrelated per-run filter threshold.)
func WithHorizonAltitude(minAlt angle.Angle) CircumpolarOption {
	return func(c *circumpolarConfig) { c.threshold, c.hasThreshold = minAlt, true }
}

// IsCircumpolar reports whether an object at declination dec never sets as
// seen from site — its altitude stays above the horizon threshold at
// every hour angle, all the way around the pole. This is closed-form
// spherical geometry (two altitude evaluations, at upper and lower
// culmination), not a numerical search — the same question
// VisibilityEvents/VisibleIntervals only answer indirectly today (an empty
// result there means either "circumpolar" or "never rises", and telling
// those apart needs a second altitude check of its own).
//
// The horizon reference defaults to site.RiseSetThreshold() (geometric,
// elevation-corrected, no atmospheric refraction); see WithRefraction and
// WithHorizonAltitude to change that.
func IsCircumpolar(dec angle.Angle, site *Site, opts ...CircumpolarOption) bool {
	minAlt, _ := circumpolarExtremes(dec, site)

	return minAlt > circumpolarThreshold(site, opts).Radians()
}

// IsNeverUp reports the complementary case: an object at declination dec
// that never rises above the horizon (or the given/overridden threshold)
// as seen from site — its altitude stays below threshold even at upper
// culmination. See IsCircumpolar for the shared options and the closed-form
// rationale.
func IsNeverUp(dec angle.Angle, site *Site, opts ...CircumpolarOption) bool {
	_, maxAlt := circumpolarExtremes(dec, site)

	return maxAlt < circumpolarThreshold(site, opts).Radians()
}

// circumpolarExtremes returns the minimum and maximum altitude (radians) an
// object at declination dec ever reaches at site's latitude — at lower and
// upper culmination (hour angle 180° and 0°) respectively, the two
// stationary points of altitude over a full rotation. Computed via asin
// directly rather than a closed-form "90 − |lat∓dec|" shortcut: that
// shortcut's sign handling breaks down outside a limited lat/dec range,
// where asin (naturally clamped to a valid altitude, ±1 input guarded
// against float rounding) is not.
func circumpolarExtremes(dec angle.Angle, site *Site) (minAlt, maxAlt float64) {
	sinLat, cosLat := math.Sincos(site.Latitude().Radians())
	sinDec, cosDec := math.Sincos(dec.Radians())

	maxAlt = math.Asin(clamp(sinLat*sinDec+cosLat*cosDec, -1, 1)) // hour angle 0 (upper culmination)
	minAlt = math.Asin(clamp(sinLat*sinDec-cosLat*cosDec, -1, 1)) // hour angle 180 (lower culmination)

	return minAlt, maxAlt
}

// circumpolarThreshold resolves the effective altitude threshold for
// IsCircumpolar/IsNeverUp from site and opts.
func circumpolarThreshold(site *Site, opts []CircumpolarOption) angle.Angle {
	var cfg circumpolarConfig

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.hasThreshold {
		return cfg.threshold
	}

	threshold := site.RiseSetThreshold()
	if cfg.refraction {
		threshold -= angle.Deg(standardRefractionCircumpolar)
	}

	return threshold
}
