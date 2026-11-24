package plan

import (
	"errors"
	"fmt"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// Tonight is what a planner asks first about one night at a site: when the
// Sun sets and rises, when each twilight ends and begins between them, and
// the Moon's part in it.
type Tonight struct {
	// Sunset and Sunrise bound the night, as SunEvents finds them: the Sun's
	// center at the site's [Site.SunRiseSetThreshold].
	Sunset, Sunrise epoch.Time

	// Civil, Nautical and Astronomical are the nights of each kind inside it,
	// nil when the Sun does not go that deep tonight, as it does not reach 18°
	// down on a summer night at 51°N.
	Civil, Nautical, Astronomical *Night

	// Moonrise and Moonset are the Moon's rise and set between Sunset and
	// Sunrise, as MoonEvents finds them, nil when it does not rise or set
	// then.
	Moonrise, Moonset *epoch.Time

	// MoonUpAtSunset reports whether the Moon is above the horizon at Sunset.
	// With Moonrise and Moonset it says when the Moon is up all night: up at
	// sunset with no Moonset is a Moon up until sunrise.
	MoonUpAtSunset bool

	// MoonIllumination is the Moon's illuminated fraction at the middle of
	// the night, from 0 at new Moon to 1 at full.
	MoonIllumination float64
}

// TonightAt returns the Tonight at site that t falls in, or the one that
// follows t when t is in daylight, as [NightOf] chooses a night but from
// sunset to sunrise: at 02:00 the night under way, at noon the coming one.
// For the night of a calendar date, pass a daytime instant of that date,
// such as local noon.
//
// Each twilight night is the one that begins after this sunset, so an
// instant between astronomical dawn and sunrise is still in the night it
// ends, as an instant before astronomical dusk is in the night it begins.
//
// Where the Sun does not set and rise again within a day, as in a polar day
// or night, it returns [ErrNoTwilight].
func TonightAt(site *Site, t epoch.Time, prov eph.Provider) (Tonight, error) {
	sunset, sunrise, err := nightAcross(t, site, prov, site.SunRiseSetThreshold().Degrees(), "sunset and sunrise")
	if err != nil {
		return Tonight{}, err
	}

	tonight := Tonight{Sunset: sunset, Sunrise: sunrise}

	for _, k := range []struct {
		kind  TwilightKind
		night **Night
	}{
		{CivilTwilight, &tonight.Civil},
		{NauticalTwilight, &tonight.Nautical},
		{AstronomicalTwilight, &tonight.Astronomical},
	} {
		n, err := NightOf(site, sunset, prov, k.kind)

		// From sunset, NightOf finds the night of that kind beginning within
		// a day. That can only be tonight's: the next night's dusk is about a
		// day after tonight's sunset plus the twilight between them.
		switch {
		case errors.Is(err, ErrNoTwilight):
			// The Sun does not go that deep tonight.
		case err != nil:
			return Tonight{}, err
		default:
			*k.night = &n
		}
	}

	if err := tonight.addMoon(site, prov); err != nil {
		return Tonight{}, err
	}

	return tonight, nil
}

// addMoon fills in the Moon: its rise and set between sunset and sunrise,
// whether it is up at sunset, which is what its last rise or set before
// sunset left it, and its illumination at the night's middle.
func (n *Tonight) addMoon(site *Site, prov eph.Provider) error {
	events, err := MoonEvents(site, Window{Start: n.Sunset.Add(unit.Days(-1)), End: n.Sunrise}, prov)
	if err != nil {
		return fmt.Errorf("plan: tonight: moon events: %w", err)
	}

	for i := range events {
		e := &events[i]

		switch {
		case e.Kind != EventRise && e.Kind != EventSet:
		case e.Time.Before(n.Sunset):
			n.MoonUpAtSunset = e.Kind == EventRise
		case e.Kind == EventRise && n.Moonrise == nil:
			at := e.Time
			n.Moonrise = &at
		case e.Kind == EventSet && n.Moonset == nil:
			at := e.Time
			n.Moonset = &at
		}
	}

	middle := n.Sunset.Add(n.Sunrise.Sub(n.Sunset) / 2)

	n.MoonIllumination, _, err = MoonIllumination(middle, prov)
	if err != nil {
		return fmt.Errorf("plan: tonight: moon illumination: %w", err)
	}

	return nil
}
