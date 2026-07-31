package plan

import (
	"fmt"
	"math"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Site represents a physical observing location.
// Sites are immutable by convention.
type Site struct {
	location       *coord.Geodetic
	timeZone       *time.Location
	name           string
	mpcCode        string
	aliases        []string
	horizon        angle.Angle
	horizonProfile HorizonProfile
}

// HorizonProfile computes the local horizon elevation limit at a given
// azimuth, for a site whose sky isn't uniformly clear down to a single
// scalar Horizon() — "my east horizon is blocked to 30° by a ridge, west
// is open to the flat scalar limit." azimuth is measured from North,
// increasing eastward, matching this package's existing azimuth
// convention.
//
// This is purely additive data plumbing today: no production call site
// consumes it yet (RiseSetThreshold and friends use HorizonDip, the
// atmospheric dip from elevation, not this) — a future Horizon
// constraint gating visibility per-azimuth (see docs/ROADMAP.md #29)
// would be the natural consumer. Set it via WithHorizonProfile;
// Site.HorizonAt falls back to the scalar Horizon() when none is set.
type HorizonProfile func(azimuth angle.Angle) angle.Angle

// KnownSites maps a modest, defensible starter list of well-known observing
// sites (not an exhaustive observatory database) to fully-built *Site
// values, keyed by a lowercase/underscore slug. Coordinates and elevations
// are the published geodetic values from each site's own Wikipedia
// infobox, cross-checked against the IAU Minor Planet Center's own
// observatory code list (https://minorplanetcenter.net/iau/lists/ObsCodesF.html)
// where a code is set. A caller needing survey-grade precision for their
// own site should always supply their own measured coordinates via
// NewSite — this table is a convenience for "somewhere near Mauna Kea,"
// not a substitute for that. See NewKnownSite for name/alias-based lookup.
var KnownSites = map[string]*Site{
	"greenwich": {
		name:     "Greenwich",
		location: coord.MustGeodetic(angle.Zero(), angle.Deg(51.4772), 45),
		timeZone: time.MustLocation("Europe/London"),
		mpcCode:  "000",
		aliases:  []string{"Royal Observatory"},
		horizon:  angle.Zero(),
	},
	"mauna_kea": {
		name:     "Mauna Kea",
		location: coord.MustGeodetic(angle.Deg(-155.47441), angle.Deg(19.8263), 4145),
		timeZone: time.MustLocation("Pacific/Honolulu"),
		mpcCode:  "568",
		aliases:  []string{"Keck", "W. M. Keck Observatory"},
		horizon:  angle.Zero(),
	},
	"paranal": {
		name:     "Paranal",
		location: coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635),
		timeZone: time.MustLocation("America/Santiago"),
		mpcCode:  "309",
		aliases:  []string{"VLT", "Cerro Paranal", "Very Large Telescope"},
		horizon:  angle.Zero(),
	},
	"la_palma": {
		name:     "La Palma",
		location: coord.MustGeodetic(angle.Deg(-17.8947), angle.Deg(28.7636), 2396),
		timeZone: time.MustLocation("Atlantic/Canary"),
		mpcCode:  "950",
		aliases:  []string{"Roque de los Muchachos", "ORM"},
		horizon:  angle.Zero(),
	},
	"cerro_tololo": {
		name:     "Cerro Tololo",
		location: coord.MustGeodetic(angle.Deg(-70.80639), angle.Deg(-30.16917), 2207),
		timeZone: time.MustLocation("America/Santiago"),
		mpcCode:  "807",
		aliases:  []string{"CTIO"},
		horizon:  angle.Zero(),
	},
	"kitt_peak": {
		name:     "Kitt Peak",
		location: coord.MustGeodetic(angle.Deg(-111.5967), angle.Deg(31.9583), 2096),
		timeZone: time.MustLocation("America/Phoenix"),
		mpcCode:  "695",
		aliases:  []string{"KPNO"},
		horizon:  angle.Zero(),
	},
	"la_silla": {
		name:     "La Silla",
		location: coord.MustGeodetic(angle.Deg(-70.7375), angle.Deg(-29.2575), 2400),
		timeZone: time.MustLocation("America/Santiago"),
		mpcCode:  "809",
		aliases:  []string{"ESO La Silla"},
		horizon:  angle.Zero(),
	},
	"siding_spring": {
		name:     "Siding Spring",
		location: coord.MustGeodetic(angle.Deg(149.06444), angle.Deg(-31.27333), 1165),
		timeZone: time.MustLocation("Australia/Sydney"),
		mpcCode:  "413",
		aliases:  []string{"SSO"},
		horizon:  angle.Zero(),
	},
	"palomar": {
		name:     "Palomar",
		location: coord.MustGeodetic(angle.Deg(-116.865), angle.Deg(33.3564), 1712),
		timeZone: time.MustLocation("America/Los_Angeles"),
		mpcCode:  "675",
		aliases:  []string{"Palomar Mountain", "Palomar Observatory"},
		horizon:  angle.Zero(),
	},
	"cerro_pachon": {
		name:     "Cerro Pachón",
		location: coord.MustGeodetic(angle.Deg(-70.74942), angle.Deg(-30.24464), 2647),
		timeZone: time.MustLocation("America/Santiago"),
		mpcCode:  "X05",
		aliases:  []string{"Rubin Observatory", "Vera C. Rubin Observatory", "LSST"},
		horizon:  angle.Zero(),
	},
}

// normalizeSiteName lowercases and replaces spaces with underscores so lookups match
// "Mauna Kea", "mauna kea", and "MaunaKea" alike.
func normalizeSiteName(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "_"))
}

// lookupKnownSite finds a KnownSites entry by its map key (the normalized
// form of its Name) or any Alias, case- and space-insensitive.
func lookupKnownSite(name string) (*Site, bool) {
	want := normalizeSiteName(name)

	if s, ok := KnownSites[want]; ok {
		return s, true
	}

	for _, s := range KnownSites {
		for _, alias := range s.aliases {
			if normalizeSiteName(alias) == want {
				return s, true
			}
		}
	}

	return nil, false
}

// NewKnownSite looks up name (matched against every KnownSites entry's
// Name and Aliases, case- and space-insensitive) and returns its *Site, or
// ErrUnknownSite if no entry matches. The registry's shared *Site is
// returned directly — a caller wanting a variant (a different horizon,
// time zone, ...) should chain the returned Site's own WithHorizon/
// WithTimeZone rather than have NewKnownSite rebuild one, since those
// already do exactly that without duplicating this lookup's logic.
func NewKnownSite(name string) (*Site, error) {
	s, ok := lookupKnownSite(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownSite, name)
	}

	return s, nil
}

// SiteOption configures optional NewSite parameters.
type SiteOption func(*Site)

// WithHorizon sets the site's local horizon limit (e.g., 0 deg for ideal,
// 20 deg for trees/hills). Defaults to angle.Zero() if omitted.
func WithHorizon(h angle.Angle) SiteOption {
	return func(s *Site) { s.horizon = h }
}

// WithHorizonProfile sets a per-azimuth horizon profile, consulted by
// Site.HorizonAt in preference to the scalar Horizon() wherever it's set.
// A nil profile (the default) makes HorizonAt fall back to Horizon() for
// every azimuth. See HorizonProfile's doc comment for the current
// (additive-only) scope.
func WithHorizonProfile(p HorizonProfile) SiteOption {
	return func(s *Site) { s.horizonProfile = p }
}

// WithTimeZone sets the site's local time zone. Defaults to UTC if omitted
// (see Site.TimeZone).
func WithTimeZone(tz *time.Location) SiteOption {
	return func(s *Site) { s.timeZone = tz }
}

// WithMPCCode sets the site's IAU Minor Planet Center observatory code
// (e.g. "568" for Mauna Kea). Purely informational metadata — see
// Site.MPCCode's doc comment for why it doesn't participate in Equal.
func WithMPCCode(code string) SiteOption {
	return func(s *Site) { s.mpcCode = code }
}

// WithSiteAliases sets additional names this site is known by (e.g. "Keck" for
// Mauna Kea). Purely informational metadata — see Site.Aliases' doc
// comment for why it doesn't participate in Equal.
func WithSiteAliases(aliases ...string) SiteOption {
	return func(s *Site) { s.aliases = aliases }
}

// NewSite creates a new observing site with validation.
// name: A human-readable name for the site.
// loc: The geodetic location (longitude, latitude, height).
// opts: optional parameters — see WithHorizon, WithTimeZone, WithMPCCode,
// and WithSiteAliases.
func NewSite(name string, loc *coord.Geodetic, opts ...SiteOption) (*Site, error) {
	if loc == nil {
		return nil, ErrNilLocation
	}

	s := &Site{
		name:     name,
		location: loc,
		horizon:  angle.Zero(),
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.horizon.Degrees() < -90 || s.horizon.Degrees() > 90 {
		return nil, ErrInvalidHorizon
	}

	return s, nil
}

// Name returns the site's human-readable name.
func (s *Site) Name() string { return s.name }

// Location returns the site's geodetic location.
func (s *Site) Location() *coord.Geodetic { return s.location }

// Horizon returns the local horizon elevation limit.
func (s *Site) Horizon() angle.Angle { return s.horizon }

// HorizonProfile returns the site's per-azimuth horizon profile, or nil if
// none was set via WithHorizonProfile.
func (s *Site) HorizonProfile() HorizonProfile { return s.horizonProfile }

// HorizonAt returns the local horizon elevation limit at azimuth: the
// profile's value if one is set via WithHorizonProfile, otherwise the
// scalar Horizon() for every azimuth.
func (s *Site) HorizonAt(azimuth angle.Angle) angle.Angle {
	if s.horizonProfile != nil {
		return s.horizonProfile(azimuth)
	}

	return s.horizon
}

// MPCCode returns the site's IAU Minor Planet Center observatory code, or
// "" if none was set. Informational metadata only — see WithMPCCode.
func (s *Site) MPCCode() string { return s.mpcCode }

// Aliases returns a copy of the additional names this site is known by, or
// nil if none were set. Informational metadata only — see WithSiteAliases.
func (s *Site) Aliases() []string {
	if s.aliases == nil {
		return nil
	}

	out := make([]string, len(s.aliases))
	copy(out, s.aliases)

	return out
}

// TimeZone returns the site's local time zone, or UTC if nil.
func (s *Site) TimeZone() *time.Location {
	if s.timeZone == nil {
		return time.LocationUTC
	}

	return s.timeZone
}

// Longitude returns the site's geodetic longitude.
func (s *Site) Longitude() angle.Angle { return s.location.Lon() }

// Latitude returns the site's geodetic latitude.
func (s *Site) Latitude() angle.Angle { return s.location.Lat() }

// HeightMeters returns the site's height above the reference ellipsoid in meters.
func (s *Site) HeightMeters() float64 { return s.location.Height() }

// Atmosphere returns an atmospheric profile adjusted for the site's elevation
// using the ICAO International Standard Atmosphere barometric formula.
// Pressure and temperature are reduced for altitude; humidity, wavelength,
// and the refraction model are inherited from the sea-level standard.
func (s *Site) Atmosphere() atmosphere.Atmosphere {
	return atmosphere.AtAltitude(s.location.Height())
}

// HorizonDip returns the geometric dip angle of the visible horizon at this
// site's elevation. At sea level the dip is zero; at 786 m it is ≈ 0.90°.
func (s *Site) HorizonDip() angle.Angle {
	return atmosphere.HorizonDip(s.location.Height())
}

// RiseSetThreshold returns the standard rise/set altitude threshold for a
// point source (star) at this site, including the geometric horizon dip
// from the site's elevation.
//
// At sea level: 0°. At 786m: −0.82° (the depressed horizon).
func (s *Site) RiseSetThreshold() angle.Angle {
	return angle.Deg(-s.HorizonDip().Degrees())
}

// SunRiseSetThreshold returns the sunrise/sunset altitude threshold.
// The Sun rises when its geometric center is at:
//
//	alt = −(semi-diameter + standard refraction + horizon dip)
//
// This matches the USNO definition (Explanatory Supplement to the
// Astronomical Almanac, §9.311):
//   - Solar semi-diameter: 16' (0.2667°)
//   - Standard atmospheric refraction at horizon: 34' (0.5667°)
//   - Horizon dip from elevation: 1.76'√h
//
// Total at sea level: −(16' + 34') = −50' = −0.8333°.
func (s *Site) SunRiseSetThreshold() angle.Angle {
	const sunSemiDiameter = 0.2667 // degrees, ~16 arcmin

	const standardRefraction = 0.5667 // degrees, ~34 arcmin

	return angle.Deg(-sunSemiDiameter - standardRefraction - s.HorizonDip().Degrees())
}

// MoonRiseSetThreshold returns the rise/set altitude threshold for the Moon.
// Follows the same convention as SunRiseSetThreshold: the Moon rises when
// its geometric center reaches:
//
//	alt = −(semi-diameter + standard refraction + horizon dip)
//
// The Moon's mean semi-diameter is ~15.5' (varies with parallax, handled
// by the topocentric correction in GeocentricToObserved).
func (s *Site) MoonRiseSetThreshold() angle.Angle {
	const moonSemiDiameter = 0.2583 // degrees, ~15.5 arcmin (mean)

	const standardRefraction = 0.5667 // degrees, ~34 arcmin

	return angle.Deg(-moonSemiDiameter - standardRefraction - s.HorizonDip().Degrees())
}

// String returns a compact representation of the site, appending the MPC
// code (when set) as a bracketed suffix.
func (s *Site) String() string {
	if s.mpcCode != "" {
		return fmt.Sprintf("Site(%s [%s]: %s, Hor=%s)", s.name, s.mpcCode, s.location, s.horizon)
	}

	return fmt.Sprintf("Site(%s: %s, Hor=%s)", s.name, s.location, s.horizon)
}

// Equal reports whether s and other represent the same observing site
// (same name, location, horizon, and time zone).
//
// MPCCode and Aliases deliberately do NOT participate in this comparison —
// they're informational metadata about which named registry entry (if
// any) a site came from, not part of its physical/observational identity.
// A hand-built Site at Mauna Kea's exact coordinates is the same *site* as
// the registry's Mauna Kea entry even without carrying its MPC code.
// HorizonProfile also does not participate — func values are not
// comparable in Go (a plain == would be a compile error), and there's no
// meaningful equality to define between two arbitrary azimuth→altitude
// functions short of calling both at every azimuth.
//
// Coordinates and horizon are compared with a tolerance of 1e-12 radians
// (~0.2 μas) to avoid false negatives from float64 round-trip drift.
func (s *Site) Equal(other *Site) bool {
	if s == nil || other == nil {
		return s == other
	}

	tzEqual := false
	if s.timeZone == nil && other.timeZone == nil {
		tzEqual = true
	} else if s.timeZone != nil && other.timeZone != nil {
		tzEqual = s.timeZone.String() == other.timeZone.String()
	}

	const eps = 1e-12 // radians, ~0.2 μas

	return s.name == other.name &&
		math.Abs(s.location.Lon().Radians()-other.location.Lon().Radians()) < eps &&
		math.Abs(s.location.Lat().Radians()-other.location.Lat().Radians()) < eps &&
		math.Abs(s.location.Height()-other.location.Height()) < eps &&
		math.Abs(s.horizon.Radians()-other.horizon.Radians()) < eps &&
		tzEqual
}

// WithHorizon returns a copy of s with the given horizon limit.
func (s *Site) WithHorizon(h angle.Angle) (*Site, error) {
	return NewSite(s.name, s.location, WithHorizon(h), WithTimeZone(s.timeZone), WithMPCCode(s.mpcCode), WithSiteAliases(s.aliases...), WithHorizonProfile(s.horizonProfile))
}

// WithTimeZone returns a copy of s with the given time zone.
func (s *Site) WithTimeZone(tz *time.Location) *Site {
	return &Site{
		name:           s.name,
		location:       s.location,
		horizon:        s.horizon,
		horizonProfile: s.horizonProfile,
		timeZone:       tz,
		mpcCode:        s.mpcCode,
		aliases:        s.Aliases(),
	}
}

// LocalSiderealTime returns the Local Apparent Sidereal Time (LAST) at the
// observer's location for the given time.
//
// LAST = GAST + east longitude
//
// It uses the IAU 2006 GAST model (Gst06a).
// Returns an error if IERS EOP data is unavailable for the UT1 conversion.
func (s *Site) LocalSiderealTime(t time.Time) (angle.Angle, error) {
	gast, err := t.GAST()
	if err != nil {
		return angle.Zero(), fmt.Errorf("LocalSiderealTime: %w", err)
	}

	lst := gast.Radians() + s.location.Lon().Radians()
	// Normalise to [0, 2π)
	lst = math.Mod(lst, 2*math.Pi)
	if lst < 0 {
		lst += 2 * math.Pi
	}

	return angle.Rad(lst), nil
}
