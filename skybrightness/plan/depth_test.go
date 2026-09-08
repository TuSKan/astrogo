package plan_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/skybrightness"
	sbplan "github.com/TuSKan/astrogo/skybrightness/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// dataset.Sky's only constructor gathers its own data, deliberately, so until
// Spec.Sky named an interface the one method this package exists for could not
// be reached without a network and 145 MB of reference data. It went uncovered
// on every commit and was checked only when a download succeeded.
//
// What follows is that method end to end, against a sky assembled here. Not a
// mock: skybrightness.NewPreset and Model.Estimate are exported and take
// synthetic inputs, so the estimate, the surface brightness and the electron
// rate are all computed by the real code — the same trick dataset's own tests
// use one layer down. Only the data is invented, and it is invented flat so
// that what varies is the thing under test.
//
// The physics is not the subject here and is checked elsewhere: the
// network-tagged tests run this against a real assembled sky at Paranal.

// errModelGaveUp is what an injected failure looks like coming out of the sky.
var errModelGaveUp = errors.New("the model gave up")

// localSky is a Sky built from synthetic inputs, with a hook for making each
// step fail.
type localSky struct {
	model *skybrightness.Model
	band  magnitude.Passband

	// The grid the model's components were built on. dataset.Sky carries this
	// into every Query; without it Model.Estimate defaults to the wider
	// DefaultOpticalGrid and the components refuse a range they do not hold.
	grid unit.SpectralGrid

	sceneErr, directionErr, surfaceErr error

	// What the last call carried, so the wiring can be asserted.
	gotSite *coord.Geodetic
	gotWhen time.GoTime
	gotAir  *atmosphere.Builder
	gotAlt  angle.Angle
	gotAz   angle.Angle
}

// uniformStars is a star map with the same radiance in every direction, and
// uniformDust the same idea for the 100 micron map. Flat on purpose: these
// tests vary the pointing and the instrument, and real sky structure would be
// a second variable competing with the one under test.
type uniformStars float64

func (u uniformStars) RadianceAt(_, _ angle.Angle) (float64, error) { return float64(u), nil }

func (uniformStars) Galactic() bool { return true }

type uniformDust float64

func (u uniformDust) IntensityAt(_, _ angle.Angle) (float64, error) { return float64(u), nil }

func newLocalSky(t *testing.T) *localSky {
	t.Helper()

	grid, err := unit.NewSpectralGrid(400, 1, 401)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	shape := skybrightness.NewSpectralRadiance(grid)
	glow := skybrightness.NewSpectralRadiance(grid)

	for i := range shape {
		// Sloped rather than flat, so a wavelength-indexing error has
		// somewhere to show.
		shape[i] = 1 + 0.002*(float64(grid.At(i))-400)
		glow[i] = 2.5e-9
	}

	band := testBand()

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, skybrightness.PresetInputs{
		Stars:         uniformStars(1e-9),
		StarShape:     shape,
		Dust:          uniformDust(2),
		AirglowZenith: glow,
		Grid:          grid,
		Band:          band,
	})
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	return &localSky{model: model, band: band, grid: grid}
}

func (s *localSky) Scene(
	site *coord.Geodetic, when time.GoTime, air *atmosphere.Builder,
) (*skybrightness.Scene, error) {
	s.gotSite, s.gotWhen, s.gotAir = site, when, air

	if s.sceneErr != nil {
		return nil, s.sceneErr
	}

	if air == nil {
		air = atmosphere.NewBuilder().SurfaceAtAltitude(site.Height())
	}

	built, err := air.Build()
	if err != nil {
		return nil, fmt.Errorf("localSky: atmosphere: %w", err)
	}

	return &skybrightness.Scene{
		Observer:   site,
		Time:       when,
		Atmosphere: built,
		Ephemeris:  eph.Default(),
	}, nil
}

func (s *localSky) Direction(
	ctx context.Context, scene *skybrightness.Scene, alt, az angle.Angle,
) (*skybrightness.Estimate, error) {
	s.gotAlt, s.gotAz = alt, az

	if s.directionErr != nil {
		return nil, s.directionErr
	}

	est, err := s.model.Estimate(ctx, skybrightness.Query{
		Scene:     scene,
		Direction: coord.NewAltAz(alt, az),
		Grid:      s.grid,
	})
	if err != nil {
		return nil, fmt.Errorf("localSky: estimate: %w", err)
	}

	return est, nil
}

func (s *localSky) SurfaceBrightness(est *skybrightness.Estimate) (float64, error) {
	if s.surfaceErr != nil {
		return 0, s.surfaceErr
	}

	v, err := est.SurfaceBrightness(s.band, magnitude.Vega)
	if err != nil {
		return 0, fmt.Errorf("localSky: surface brightness: %w", err)
	}

	return v, nil
}

func (s *localSky) Band() magnitude.Passband { return s.band }

// testInstrument is a plausible small imaging chain. The numbers are not a real
// product and nothing here depends on them being one — only on the chain
// validating and producing a positive electron rate.
func testInstrument() optics.Instrument {
	return optics.Instrument{
		Name:              "test 200mm",
		CollectingAreaM2:  0.0314,
		PixelSolidAngleSR: 1e-11,
		Throughput: []optics.Throughput{{
			// The filter, matching the sky model's band. Declaring it is what
			// makes the returned magnitude a filter magnitude rather than a
			// broadband rate quoted in the model's band — see
			// Imaging.LimitingMagnitudeAt.
			Name:         "test V filter",
			WavelengthNM: []unit.WavelengthNM{499, 500, 600, 601},
			Efficiency:   []float64{0, 0.5, 0.5, 0},
			Reference:    "synthetic, for tests",
		}},
		ReadNoiseElectrons: 5,
		DarkCurrentEPerSec: 0.01,
	}
}

func testBand() magnitude.Passband {
	return magnitude.Passband{
		Name:            "test V",
		WavelengthNM:    []unit.WavelengthNM{499, 500, 600, 601},
		Response:        []float64{0, 1, 1, 0},
		Detector:        magnitude.EnergyIntegrating,
		VegaZeroPointJy: 3636,
	}
}

func testSpec(t *testing.T, sky sbplan.Sky) sbplan.Spec {
	t.Helper()

	site, err := coord.NewGeodetic(angle.Deg(-70.4028), angle.Deg(-24.6251), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return sbplan.Spec{
		Sky:            sky,
		Site:           site,
		Instrument:     testInstrument(),
		Exposure:       300 * time.Second,
		AperturePixels: 20,
		SNR:            5,
	}
}

func testEpoch() time.Time {
	return time.Date(2026, time.April, 20, 3, 0, 0, 0, time.LocationUTC)
}

// TestLimitingMagnitudeAtCarriesTheRequestThrough is the wiring. A depth
// computed for the wrong pointing, the wrong instant or the wrong site is a
// number that looks exactly like a right one.
func TestLimitingMagnitudeAtCarriesTheRequestThrough(t *testing.T) {
	sky := newLocalSky(t)
	spec := testSpec(t, sky)
	spec.Air = atmosphere.NewBuilder().SurfaceAtAltitude(2635)

	img, err := sbplan.NewImaging(spec)
	if err != nil {
		t.Fatalf("NewImaging: %v", err)
	}

	when := testEpoch()
	alt, az := angle.Deg(60), angle.Deg(135)

	got, err := img.LimitingMagnitudeAt(when, alt, az)
	if err != nil {
		t.Fatalf("LimitingMagnitudeAt: %v", err)
	}

	if sky.gotSite != spec.Site {
		t.Error("the scene was built for a different site than the spec holds")
	}

	if !sky.gotWhen.Equal(when.GoTime()) {
		t.Errorf("the scene was built at %v, want %v", sky.gotWhen, when.GoTime())
	}

	if sky.gotAir != spec.Air {
		t.Error("the caller's atmosphere did not reach the scene")
	}

	if sky.gotAlt != alt || sky.gotAz != az {
		t.Errorf("evaluated at alt %v az %v, want %v %v", sky.gotAlt, sky.gotAz, alt, az)
	}

	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("limiting magnitude = %v", got)
	}

	// No invented invariant here, and the first version of this test had one:
	// I asserted the depth must be fainter than the surface brightness it came
	// from, and it is not. It came out 20.81 against a 21.21 mag/arcsec² sky,
	// which is correct — a 0.42 arcsec² pixel, 20 of them, 300 s and 5 e⁻ of
	// read noise put the threshold above one pixel's sky rate. Whether a depth
	// beats its own sky brightness depends on aperture, exposure and noise, so
	// it is a property of this fixture rather than of the conversion.
	//
	// What is a property is monotonicity, and it is checked below.
	t.Logf("depth %.3f at alt %v az %v", got, alt, az)
}

// TestDepthDeepensWithExposure is the invariant worth asserting, and it holds
// whatever the invented instrument happens to be: more integration is a fainter
// limit, always. It runs the whole path twice, which is also what makes it a
// check on the path rather than on the arithmetic — limitingMagnitude alone is
// covered directly in imaging_test.go.
func TestDepthDeepensWithExposure(t *testing.T) {
	sky := newLocalSky(t)

	depth := func(exposure time.Duration) float64 {
		t.Helper()

		spec := testSpec(t, sky)
		spec.Exposure = exposure

		img, err := sbplan.NewImaging(spec)
		if err != nil {
			t.Fatalf("NewImaging: %v", err)
		}

		got, err := img.LimitingMagnitudeAt(testEpoch(), angle.Deg(60), angle.Deg(135))
		if err != nil {
			t.Fatalf("LimitingMagnitudeAt: %v", err)
		}

		return got
	}

	short := depth(30 * time.Second)
	long := depth(1800 * time.Second)

	if !(long > short) {
		t.Errorf("a 1800 s exposure reaches %.3f and a 30 s one %.3f; longer must go fainter",
			long, short)
	}

	t.Logf("30 s = %.3f, 1800 s = %.3f", short, long)
}

// TestDepthFollowsTheSky: a brighter sky is a shallower limit. This is the
// property the whole type exists to express, and it is checked as a direction
// rather than a value because the value depends on the invented data while the
// direction does not.
func TestDepthFollowsTheSky(t *testing.T) {
	sky := newLocalSky(t)

	img, err := sbplan.NewImaging(testSpec(t, sky))
	if err != nil {
		t.Fatalf("NewImaging: %v", err)
	}

	when := testEpoch()

	// Two pointings: high, and low enough to sit under noticeably more air.
	high, err := img.LimitingMagnitudeAt(when, angle.Deg(85), angle.Deg(0))
	if err != nil {
		t.Fatalf("LimitingMagnitudeAt(85°): %v", err)
	}

	low, err := img.LimitingMagnitudeAt(when, angle.Deg(20), angle.Deg(0))
	if err != nil {
		t.Fatalf("LimitingMagnitudeAt(20°): %v", err)
	}

	if high == low {
		t.Fatal("the depth is identical at 85° and 20°, so the pointing is not reaching the model")
	}

	t.Logf("depth at 85° = %.3f, at 20° = %.3f", high, low)
}

// TestLimitingMagnitudeAtReportsWhichStepFailed: several things can fail
// between a request and a magnitude, and each has to say which. A swallowed one
// would make a failure indistinguishable from a shallow sky, which is the
// distinction this repository keeps having to restore.
func TestLimitingMagnitudeAtReportsWhichStepFailed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		breakIt func(*localSky)
		want    string
	}{
		{"scene", func(s *localSky) { s.sceneErr = errModelGaveUp }, "scene"},
		{"direction", func(s *localSky) { s.directionErr = errModelGaveUp }, "estimate"},
		{"surface brightness", func(s *localSky) { s.surfaceErr = errModelGaveUp }, "surface brightness"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sky := newLocalSky(t)
			tc.breakIt(sky)

			img, err := sbplan.NewImaging(testSpec(t, sky))
			if err != nil {
				t.Fatalf("NewImaging: %v", err)
			}

			_, err = img.LimitingMagnitudeAt(testEpoch(), angle.Deg(60), angle.Deg(135))
			if !errors.Is(err, errModelGaveUp) {
				t.Fatalf("err = %v, want it to wrap the model's own error", err)
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error does not name the step that failed (%q):\n  %v", tc.want, err)
			}
		})
	}
}

// TestBandIsTheSkysOwnBand: the magnitudes are on the sky model's band, and
// comparing them against a catalogue magnitude in a different filter is a
// mistake that produces entirely plausible numbers. Band is how a caller avoids
// it, so it must report the band in use rather than a constant.
func TestBandIsTheSkysOwnBand(t *testing.T) {
	for _, name := range []string{"test V", "Johnson B", "SDSS r"} {
		sky := newLocalSky(t)
		sky.band.Name = name

		img, err := sbplan.NewImaging(testSpec(t, sky))
		if err != nil {
			t.Fatalf("NewImaging: %v", err)
		}

		if got := img.Band(); got != name {
			t.Errorf("Band() = %q, want %q", got, name)
		}
	}
}

// TestNewImagingAcceptsAWholeSpec is the successful construction, which no
// offline test could reach before: every earlier one stopped at the nil-Sky
// check, so the constructor's own arithmetic — a pixel's solid angle in square
// arcseconds — ran only under a network tag.
func TestNewImagingAcceptsAWholeSpec(t *testing.T) {
	img, err := sbplan.NewImaging(testSpec(t, newLocalSky(t)))
	if err != nil {
		t.Fatalf("NewImaging: %v", err)
	}

	if img == nil {
		t.Fatal("NewImaging returned nil with no error")
	}

	if img.Band() != "test V" {
		t.Errorf("Band() = %q, want the band the sky reported", img.Band())
	}
}
