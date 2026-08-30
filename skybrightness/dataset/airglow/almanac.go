package airglow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/time"
)

// Almanac is what SkyCalc's almanac knows about an instant that an airglow
// request needs.
//
// All three fields are things [Spec] otherwise asks a caller to supply from
// somewhere else, and all three are functions of the date alone.
type Almanac struct {
	// SolarFluxSFU is the monthly averaged 10.7 cm solar radio flux for the
	// month containing the instant, in solar flux units.
	//
	// Zero means the almanac has no published average for that month, which
	// is the ordinary answer for the current one and for anything ahead of
	// it: the figure comes from Natural Resources Canada and is a monthly
	// mean, so it does not exist until the month is over. Zero is chosen
	// deliberately because it is also what [Spec.SolarFluxSFU] treats as
	// unset, so the two compose — an almanac lookup that came back empty
	// leaves the request at SkyCalc's own default rather than at the -1 the
	// service actually returns, which would be a negative solar flux.
	SolarFluxSFU float64

	// Season is the bimonthly period, 1 for December-January through 6 for
	// October-November, ready for [Spec.Season].
	Season int

	// TimeOfNight is the third of the night, 1 to 3, ready for
	// [Spec.TimeOfNight]. It is computed for the instant given, so an
	// instant outside the night still returns a third.
	TimeOfNight int
}

// AlmanacAt asks SkyCalc's almanac what it knows about an instant.
//
// # What this replaces
//
// Three lookups a caller would otherwise do by hand. The solar flux comes
// from Natural Resources Canada's published monthly means, which
// [Spec.SolarFluxSFU] currently tells a caller to go and find; the season and
// third of night are the two integers whose effect on airglow is a factor of
// one and a half and which nobody guesses correctly from a date.
//
// So the intended use is to let the date decide:
//
//	a, err := airglow.AlmanacAt(ctx, when, airglow.Paranal)
//	spec := airglow.Spec{
//		Observatory:  airglow.Paranal,
//		SolarFluxSFU: a.SolarFluxSFU,
//		Season:       a.Season,
//		TimeOfNight:  a.TimeOfNight,
//	}
//
// # What it deliberately does not return
//
// The almanac also computes airmass, the Moon's position and separation, and
// the ecliptic coordinates of a target. None of them belong in an airglow
// request: this package asks SkyCalc for the zenith spectrum at airmass 1
// because the component applies van Rhijn itself, and it switches every
// non-airglow component off. Returning them would invite a caller to feed
// back geometry that would then be counted twice.
//
// Coordinates are not taken for the same reason. The almanac requires a
// target right ascension and declination to compute the parts this does not
// use, so zero is sent; the three fields above depend on the instant and the
// site, not on where the telescope points.
//
// # Not cached
//
// One small JSON call against a service this package already talks to, and
// the answer for a past month never changes — but a caller assembling one
// scene makes this call once, so there is nothing here worth the cache key.
func AlmanacAt(ctx context.Context, when time.GoTime, obs Observatory) (Almanac, error) {
	if obs == "" {
		obs = Paranal
	}

	switch obs {
	case Paranal, LaSilla, Armazones, Altitude3060:
	default:
		return Almanac{}, fmt.Errorf("%w: observatory %q is not one SkyCalc models", ErrSpec, obs)
	}

	utc := when.UTC()

	req := almanacRequest{
		RA:          0,
		Dec:         0,
		InputType:   "ut_time",
		Year:        utc.Year(),
		Month:       int(utc.Month()),
		Day:         utc.Day(),
		Hour:        utc.Hour(),
		Minute:      utc.Minute(),
		Second:      utc.Second(),
		Observatory: string(obs),
	}

	client, err := api.NewClient(remote.ESOSkyCalc)
	if err != nil {
		return Almanac{}, fmt.Errorf("airglow: almanac client: %w", err)
	}

	defer func() { _ = client.Close() }()

	body, err := client.PostJSON(ctx, remote.ESOSkyCalc, "api/skycalc_almanac", req)
	if err != nil {
		return Almanac{}, fmt.Errorf("airglow: almanac request: %w", err)
	}

	var res almanacResponse

	decodeErr := json.NewDecoder(body).Decode(&res)
	_ = body.Close()

	if decodeErr != nil {
		return Almanac{}, fmt.Errorf("%w: decoding the almanac response: %w", ErrService, decodeErr)
	}

	return res.almanac()
}

// almanacRequest is the JSON body the almanac takes.
type almanacRequest struct {
	RA          float64 `json:"coord_ra"`
	Dec         float64 `json:"coord_dec"`
	InputType   string  `json:"input_type"`
	Year        int     `json:"coord_year"`
	Month       int     `json:"coord_month"`
	Day         int     `json:"coord_day"`
	Hour        int     `json:"coord_ut_hour"`
	Minute      int     `json:"coord_ut_min"`
	Second      int     `json:"coord_ut_sec"`
	Observatory string  `json:"observatory"`
}

// almanacResponse is the part of the reply this package reads.
type almanacResponse struct {
	Output struct {
		Observation struct {
			SeasonFlag int `json:"season_flag"`
			TimeFlag   int `json:"time_flag"`
		} `json:"observation"`

		Sun struct {
			AveFlux float64 `json:"sun_aveflux"`
		} `json:"sun"`
	} `json:"output"`
}

// almanac turns the reply into the three fields, checking each against the
// range SkyCalc's own request parameters accept.
func (r almanacResponse) almanac() (Almanac, error) {
	season := r.Output.Observation.SeasonFlag
	night := r.Output.Observation.TimeFlag

	if season < 0 || season > 6 {
		return Almanac{}, fmt.Errorf("%w: almanac returned season %d", ErrService, season)
	}

	if night < 0 || night > 3 {
		return Almanac{}, fmt.Errorf("%w: almanac returned time of night %d", ErrService, night)
	}

	// The service says "no published average" with -1. Anything else
	// negative, or a NaN, is a response this package should not pass on as a
	// solar flux.
	flux := r.Output.Sun.AveFlux

	switch {
	case flux == -1:
		flux = 0
	case math.IsNaN(flux) || math.IsInf(flux, 0) || flux < 0:
		return Almanac{}, fmt.Errorf("%w: almanac returned a solar flux of %g sfu",
			ErrService, flux)
	}

	return Almanac{SolarFluxSFU: flux, Season: season, TimeOfNight: night}, nil
}
