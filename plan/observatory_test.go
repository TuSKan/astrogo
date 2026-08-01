package plan

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"

	"github.com/TuSKan/astrogo/time"
)

func TestNewSite(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	tz, _ := time.LoadLocation("Europe/Rome")
	horizon := angle.Deg(20)

	site, err := NewSite("My Observatory", loc, WithHorizon(horizon), WithTimeZone(tz))
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "Name", site.Name(), "My Observatory")
	testutil.AssertEqual(t, "Longitude", site.Longitude().Degrees(), 10.0)
	testutil.AssertEqual(t, "Latitude", site.Latitude().Degrees(), 45.0)
	testutil.AssertEqual(t, "Height", site.HeightMeters(), 500.0)
	testutil.AssertEqual(t, "Horizon", site.Horizon().Degrees(), 20.0)
	testutil.AssertEqual(t, "TimeZone", site.TimeZone().String(), "Europe/Rome")
}

// TestNewSiteEarthLocation confirms the plain lat/lon/height convenience
// constructor produces the same Site a caller would get by hand-building a
// coord.Geodetic via coord.NewEarthLocation + NewSite — the whole point is
// that a caller no longer needs to import coord for this common case.
func TestNewSiteEarthLocation(t *testing.T) {
	site, err := NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "Name", site.Name(), "Quinta Calixto")
	testutil.AssertEqual(t, "Latitude", site.Latitude().Degrees(), -22.528478)
	testutil.AssertEqual(t, "Longitude", site.Longitude().Degrees(), -46.473002)
	testutil.AssertEqual(t, "Height", site.HeightMeters(), 835.05)
}

// TestNewSiteEarthLocation_InvalidLatitude confirms the underlying
// coord.NewEarthLocation validation error still surfaces (not swallowed).
func TestNewSiteEarthLocation_InvalidLatitude(t *testing.T) {
	if _, err := NewSiteEarthLocation("Bad", 200, 0, 0); err == nil {
		t.Error("expected an error for a latitude outside [-90, 90]")
	}
}

// TestNewSiteEarthAddress_Offline confirms remote.SetOffline's ErrOffline
// surfaces through NewSiteEarthAddress rather than being swallowed — the
// same offline-mode gate every other remote.KindAPI caller respects.
func TestNewSiteEarthAddress_Offline(t *testing.T) {
	t.Cleanup(remote.Capture(remote.Nominatim).Restore)
	remote.SetOffline(true)

	_, err := NewSiteEarthAddress(context.Background(), "Quinta Calixto", "Extrema, MG, Brazil")
	if !errors.Is(err, remote.ErrOffline) {
		t.Errorf("NewSiteEarthAddress while offline: got %v, want wrapping remote.ErrOffline", err)
	}
}

// jsonServer starts an httptest.Server that always responds with body as a
// JSON payload — the same offline-mock shape catalog/sbdb's own tests use
// (remote.SetURL pointed at a local server instead of the real endpoint).
func jsonServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, body); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestNewSiteEarthAddress_Success exercises the full geocode + elevation
// round trip against mocked Nominatim/Open-Elevation responses — the real
// live services are covered separately by the network-tagged
// TestNewSiteEarthAddress_Live.
func TestNewSiteEarthAddress_Success(t *testing.T) {
	geo := jsonServer(t, `[{"lat":"-22.528478","lon":"-46.473002"}]`)
	elev := jsonServer(t, `{"results":[{"elevation":835.05}]}`)

	t.Cleanup(remote.Capture(remote.Nominatim, remote.OpenElevation).Restore)

	if err := remote.SetURL(remote.Nominatim, geo.URL); err != nil {
		t.Fatal(err)
	}

	if err := remote.SetURL(remote.OpenElevation, elev.URL); err != nil {
		t.Fatal(err)
	}

	site, err := NewSiteEarthAddress(context.Background(), "Quinta Calixto", "Extrema, MG, Brazil")
	if err != nil {
		t.Fatalf("NewSiteEarthAddress: %v", err)
	}

	testutil.AssertEqual(t, "Name", site.Name(), "Quinta Calixto")
	testutil.AssertEqual(t, "Latitude", site.Latitude().Degrees(), -22.528478)
	testutil.AssertEqual(t, "Longitude", site.Longitude().Degrees(), -46.473002)
	testutil.AssertEqual(t, "Height", site.HeightMeters(), 835.05)
}

// TestNewSiteEarthAddress_NoResult confirms ErrGeocodeNoResult surfaces
// whether the empty result comes from Nominatim (no match for the address)
// or from Open-Elevation (no elevation for otherwise-valid coordinates).
func TestNewSiteEarthAddress_NoResult(t *testing.T) {
	cases := []struct {
		name           string
		geocodeBody    string
		elevationBody  string
		skipElevServer bool
	}{
		{"GeocodeEmpty", `[]`, "", true},
		{"ElevationEmpty", `[{"lat":"-22.528478","lon":"-46.473002"}]`, `{"results":[]}`, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			geo := jsonServer(t, c.geocodeBody)

			t.Cleanup(remote.Capture(remote.Nominatim, remote.OpenElevation).Restore)

			if err := remote.SetURL(remote.Nominatim, geo.URL); err != nil {
				t.Fatal(err)
			}

			if !c.skipElevServer {
				elev := jsonServer(t, c.elevationBody)
				if err := remote.SetURL(remote.OpenElevation, elev.URL); err != nil {
					t.Fatal(err)
				}
			}

			_, err := NewSiteEarthAddress(context.Background(), "Nowhere", "a place that doesn't exist")
			if !errors.Is(err, ErrGeocodeNoResult) {
				t.Errorf("got %v, want wrapping ErrGeocodeNoResult", err)
			}
		})
	}
}

// TestNewSiteEarthAddress_BadCoordinates confirms a malformed lat/lon in
// Nominatim's response surfaces as an error rather than a silently-zeroed
// coordinate.
func TestNewSiteEarthAddress_BadCoordinates(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"BadLatitude", `[{"lat":"not-a-number","lon":"-46.473002"}]`},
		{"BadLongitude", `[{"lat":"-22.528478","lon":"not-a-number"}]`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			geo := jsonServer(t, c.body)

			t.Cleanup(remote.Capture(remote.Nominatim).Restore)

			if err := remote.SetURL(remote.Nominatim, geo.URL); err != nil {
				t.Fatal(err)
			}

			if _, err := NewSiteEarthAddress(context.Background(), "Bad", "somewhere"); err == nil {
				t.Error("expected a parse error for a non-numeric coordinate")
			}
		})
	}
}

func TestDefaultTimeZone(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	site, _ := NewSite("UTC Site", loc)

	testutil.AssertEqual(t, "Default TZ", site.TimeZone().String(), "UTC")
}

func TestInvalidHorizon(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)

	_, err := NewSite("Bad Horizon", loc, WithHorizon(angle.Deg(100)))
	if !errors.Is(err, ErrInvalidHorizon) {
		t.Errorf("Expected ErrInvalidHorizon, got %v", err)
	}

	_, err = NewSite("Bad Horizon Low", loc, WithHorizon(angle.Deg(-95)))
	if !errors.Is(err, ErrInvalidHorizon) {
		t.Errorf("Expected ErrInvalidHorizon, got %v", err)
	}
}

func TestNilLocation(t *testing.T) {
	_, err := NewSite("No Location", nil)
	if !errors.Is(err, ErrNilLocation) {
		t.Errorf("Expected ErrNilLocation, got %v", err)
	}
}

func TestString(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	site, _ := NewSite("Test", loc, WithHorizon(angle.Deg(20)))

	s := site.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestSiteEqual(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	a, _ := NewSite("Test", loc, WithHorizon(angle.Deg(20)))
	b, _ := NewSite("Test", loc, WithHorizon(angle.Deg(20)))
	c, _ := NewSite("Other", loc, WithHorizon(angle.Deg(20)))

	if !a.Equal(b) {
		t.Error("identical sites should be equal")
	}

	if a.Equal(c) {
		t.Error("sites with different names should not be equal")
	}
}

func TestWithHorizon(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	site, _ := NewSite("Test", loc)

	s2, err := site.WithHorizon(angle.Deg(15))
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "WithHorizon", s2.Horizon().Degrees(), 15.0, 1e-12)

	// Invalid horizon should fail
	_, err = site.WithHorizon(angle.Deg(100))
	testutil.AssertError(t, err)
}

// TestHorizonAt_FallsBackToScalar confirms HorizonAt returns the plain
// scalar Horizon() at every azimuth when no profile was set — the
// zero-value (nil HorizonProfile) case every existing Site already has.
func TestHorizonAt_FallsBackToScalar(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	site, _ := NewSite("Test", loc, WithHorizon(angle.Deg(12)))

	if site.HorizonProfile() != nil {
		t.Errorf("HorizonProfile() = %v, want nil (none set)", site.HorizonProfile())
	}

	for _, az := range []float64{0, 90, 180, 270, 359} {
		got := site.HorizonAt(angle.Deg(az)).Degrees()
		if math.Abs(got-12) > 1e-12 {
			t.Errorf("HorizonAt(%g°) = %v, want 12 (scalar fallback)", az, got)
		}
	}
}

// TestHorizonAt_ProfileWinsOverScalar confirms a set profile takes
// precedence over the scalar Horizon() wherever HorizonAt is consulted.
func TestHorizonAt_ProfileWinsOverScalar(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)

	// A ridge blocking the eastern sky to 30°, clear (0°) everywhere else.
	profile := func(az angle.Angle) angle.Angle {
		if az.Degrees() >= 45 && az.Degrees() <= 135 {
			return angle.Deg(30)
		}

		return angle.Zero()
	}

	site, _ := NewSite("Test", loc, WithHorizon(angle.Deg(5)), WithHorizonProfile(profile))

	if got := site.HorizonAt(angle.Deg(90)).Degrees(); math.Abs(got-30) > 1e-12 {
		t.Errorf("HorizonAt(90°) = %v, want 30 (profile, not the 5° scalar)", got)
	}

	if got := site.HorizonAt(angle.Deg(270)).Degrees(); math.Abs(got-0) > 1e-12 {
		t.Errorf("HorizonAt(270°) = %v, want 0 (profile, not the 5° scalar)", got)
	}

	// The plain scalar accessor is untouched by the profile.
	if got := site.Horizon().Degrees(); math.Abs(got-5) > 1e-12 {
		t.Errorf("Horizon() = %v, want 5 (unchanged scalar)", got)
	}
}

// TestHorizonProfile_SurvivesCopyConstructors is a regression test for the
// class of bug where a new Site field is silently dropped by one of the
// two copy-with methods — both WithHorizon (rebuilds via NewSite+options)
// and WithTimeZone (a hand-copied struct literal) must carry the profile
// forward.
func TestHorizonProfile_SurvivesCopyConstructors(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	profile := func(angle.Angle) angle.Angle { return angle.Deg(7) }

	site, _ := NewSite("Test", loc, WithHorizonProfile(profile))
	if site.HorizonProfile() == nil {
		t.Fatal("HorizonProfile() = nil right after construction, want the set profile")
	}

	viaHorizon, err := site.WithHorizon(angle.Deg(3))
	testutil.AssertNoError(t, err)

	if viaHorizon.HorizonProfile() == nil {
		t.Error("WithHorizon dropped the horizon profile")
	} else if got := viaHorizon.HorizonAt(angle.Zero()).Degrees(); math.Abs(got-7) > 1e-12 {
		t.Errorf("WithHorizon-copied HorizonAt(0°) = %v, want 7 (from the surviving profile)", got)
	}

	tz, _ := time.LoadLocation("Europe/Rome")
	viaTimeZone := site.WithTimeZone(tz)

	if viaTimeZone.HorizonProfile() == nil {
		t.Error("WithTimeZone dropped the horizon profile")
	} else if got := viaTimeZone.HorizonAt(angle.Zero()).Degrees(); math.Abs(got-7) > 1e-12 {
		t.Errorf("WithTimeZone-copied HorizonAt(0°) = %v, want 7 (from the surviving profile)", got)
	}
}

// TestHorizonProfile_AzimuthWrap confirms a profile is consulted correctly
// right at the 0°/360° azimuth wrap boundary — a plausible off-by-wrap bug
// site for any future CSV/DEM-backed profile implementation to trip on.
func TestHorizonProfile_AzimuthWrap(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)

	// Blocked due north (a wrap-straddling band from 350° through 10°).
	profile := func(az angle.Angle) angle.Angle {
		d := az.Degrees()
		if d >= 350 || d <= 10 {
			return angle.Deg(25)
		}

		return angle.Zero()
	}

	site, _ := NewSite("Test", loc, WithHorizonProfile(profile))

	for _, az := range []float64{0, 5, 355, 359.9} {
		if got := site.HorizonAt(angle.Deg(az)).Degrees(); math.Abs(got-25) > 1e-9 {
			t.Errorf("HorizonAt(%g°) = %v, want 25 (inside the wrap-straddling band)", az, got)
		}
	}

	if got := site.HorizonAt(angle.Deg(180)).Degrees(); math.Abs(got-0) > 1e-12 {
		t.Errorf("HorizonAt(180°) = %v, want 0 (outside the band)", got)
	}
}

func TestLocalSiderealTime(t *testing.T) {
	// Greenwich (lon=0) at J2000.0 (2000-01-01 12:00:00 UTC = JD 2451545.0)
	// GAST at J2000.0 is approximately 18.697 hours = 280.46 degrees
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(51.5), 0) // Greenwich
	site, _ := NewSite("Greenwich", loc)

	tm := time.FromJD(2451545.0, time.UTC)

	lst, err := site.LocalSiderealTime(tm)
	if err != nil {
		t.Fatalf("LocalSiderealTime failed: %v", err)
	}

	// Known GAST at J2000.0 ~280.46° ± 0.5°
	expectedDeg := 280.46
	testutil.AssertNear(t, "LST at Greenwich J2000", lst.Degrees(), expectedDeg, 0.5)
}

// TestKnownSitesTableIntegrity guards KnownSites' fixed data table against
// copy-paste mistakes: no duplicate names/aliases/MPC codes and every
// coordinate in a physically valid range.
func TestKnownSitesTableIntegrity(t *testing.T) {
	seenName := make(map[string]string)
	seenCode := make(map[string]string)

	if len(KnownSites) == 0 {
		t.Fatal("expected a non-empty starter list of known sites")
	}

	for _, s := range KnownSites {
		if s.Name() == "" {
			t.Errorf("a KnownSites entry has an empty Name")
		}

		norm := strings.ToLower(strings.ReplaceAll(s.Name(), " ", ""))
		if prev, ok := seenName[norm]; ok {
			t.Errorf("name %q collides with %q", s.Name(), prev)
		}

		seenName[norm] = s.Name()

		for _, alias := range s.Aliases() {
			an := strings.ToLower(strings.ReplaceAll(alias, " ", ""))
			if prev, ok := seenName[an]; ok {
				t.Errorf("%s: alias %q collides with %q", s.Name(), alias, prev)
			}

			seenName[an] = s.Name()
		}

		if code := s.MPCCode(); code != "" {
			if prev, ok := seenCode[code]; ok {
				t.Errorf("MPC code %q used by both %q and %q", code, prev, s.Name())
			}

			seenCode[code] = s.Name()
		}

		if lat := s.Latitude().Degrees(); lat < -90 || lat > 90 {
			t.Errorf("%s: Latitude=%v out of range [-90,90]", s.Name(), lat)
		}

		if lon := s.Longitude().Degrees(); lon < -180 || lon > 180 {
			t.Errorf("%s: Longitude=%v out of range [-180,180]", s.Name(), lon)
		}

		if h := s.HeightMeters(); h < 0 || h > 6000 {
			t.Errorf("%s: HeightMeters=%v outside a plausible ground-observatory range", s.Name(), h)
		}
	}
}

// TestNewKnownSiteCaseAndAliasInsensitive verifies the documented matching
// behavior directly, against real entries rather than a synthetic fixture.
func TestNewKnownSiteCaseAndAliasInsensitive(t *testing.T) {
	cases := []string{"Mauna Kea", "mauna kea", "mauna_kea", "MAUNA_KEA", "Keck", "keck"}

	for _, name := range cases {
		s, err := NewKnownSite(name)
		if err != nil {
			t.Errorf("NewKnownSite(%q): %v", name, err)
			continue
		}

		if s.Name() != "Mauna Kea" {
			t.Errorf("NewKnownSite(%q).Name() = %q, want %q", name, s.Name(), "Mauna Kea")
		}
	}

	if _, err := NewKnownSite("Nonexistent Observatory"); !errors.Is(err, ErrUnknownSite) {
		t.Errorf("NewKnownSite(nonexistent) error = %v, want ErrUnknownSite", err)
	}
}

// TestKnownSiteElevationSpotChecks cross-checks a couple of well-known,
// independently-verifiable elevations, guarding against a transposed digit
// slipping through the range check above unnoticed.
func TestKnownSiteElevationSpotChecks(t *testing.T) {
	want := map[string]float64{
		"Mauna Kea": 4145,
		"Paranal":   2635,
	}

	for name, wantM := range want {
		s, err := NewKnownSite(name)
		if err != nil {
			t.Fatalf("NewKnownSite(%q): %v", name, err)
		}

		if math.Abs(s.HeightMeters()-wantM) > 1 {
			t.Errorf("%s: HeightMeters() = %v, want %v", name, s.HeightMeters(), wantM)
		}
	}
}

// TestNewKnownSiteUnknownName verifies the ErrUnknownSite sentinel path.
func TestNewKnownSiteUnknownName(t *testing.T) {
	if _, err := NewKnownSite("Nowhere Observatory"); !errors.Is(err, ErrUnknownSite) {
		t.Errorf("NewKnownSite(unknown name) error = %v, want ErrUnknownSite", err)
	}
}

// TestNewKnownSite_WithHorizonVariant confirms a caller wanting a variant
// of a known site (e.g. a custom horizon for their own dome) can chain the
// returned *Site's own WithHorizon rather than needing NewKnownSite itself
// to accept options.
func TestNewKnownSite_WithHorizonVariant(t *testing.T) {
	base, err := NewKnownSite("Greenwich")
	if err != nil {
		t.Fatalf("NewKnownSite: %v", err)
	}

	site, err := base.WithHorizon(angle.Deg(15))
	if err != nil {
		t.Fatalf("WithHorizon: %v", err)
	}

	if site.Name() != "Greenwich" {
		t.Errorf("Name() = %q, want %q", site.Name(), "Greenwich")
	}

	if site.Latitude().Degrees() < 51 || site.Latitude().Degrees() > 52 {
		t.Errorf("Latitude() = %v, want ~51.48°", site.Latitude().Degrees())
	}

	if site.MPCCode() != "000" {
		t.Errorf("MPCCode() = %q, want %q", site.MPCCode(), "000")
	}

	if math.Abs(site.Horizon().Degrees()-15) > 1e-9 {
		t.Errorf("Horizon() = %v, want 15", site.Horizon().Degrees())
	}
}

// TestKnownSite_MPCCodeAndAliasesPopulated confirms a *Site built from the
// registry actually carries MPCCode/Aliases.
func TestKnownSite_MPCCodeAndAliasesPopulated(t *testing.T) {
	s, err := NewKnownSite("Mauna Kea")
	if err != nil {
		t.Fatalf("NewKnownSite: %v", err)
	}

	if s.MPCCode() != "568" {
		t.Errorf("MPCCode() = %q, want %q", s.MPCCode(), "568")
	}

	found := false

	for _, a := range s.Aliases() {
		if a == "Keck" {
			found = true
		}
	}

	if !found {
		t.Errorf("Aliases() = %v, want it to include %q", s.Aliases(), "Keck")
	}
}

// TestNewKnownSiteReturnsSharedInstanceWithoutOpts confirms the no-opts
// fast path returns the registry's own *Site rather than an unnecessary
// rebuilt copy.
func TestNewKnownSiteReturnsSharedInstanceWithoutOpts(t *testing.T) {
	s1, err := NewKnownSite("Greenwich")
	if err != nil {
		t.Fatalf("NewKnownSite: %v", err)
	}

	s2, err := NewKnownSite("greenwich")
	if err != nil {
		t.Fatalf("NewKnownSite: %v", err)
	}

	if s1 != s2 {
		t.Errorf("NewKnownSite with no opts should return the shared registry *Site, got distinct pointers")
	}
}
