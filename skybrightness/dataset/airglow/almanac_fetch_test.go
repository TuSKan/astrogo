package airglow_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
	"github.com/TuSKan/astrogo/time"
)

// AlmanacAt's request path, against a fake SkyCalc rather than Garching.
//
// It was covered only by a network-tagged test, which runs when someone
// remembers to and ESO is up — so never in CI, and never at all when ESO is
// down. Pointing the endpoint at an httptest server exercises the same code
// offline: the path the request goes to, the payload it carries, and the
// response it turns into an Almanac.

// serveAlmanac stands up a fake SkyCalc and points the endpoint at it.
func serveAlmanac(t *testing.T, status int, body string) (gotPath *string) {
	t.Helper()

	var path string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)

		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(srv.Close)

	scope := remote.Capture(remote.ESOSkyCalc)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.ESOSkyCalc, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	return &path
}

func TestAlmanacAtPostsToSkyCalcAndDecodes(t *testing.T) {
	gotPath := serveAlmanac(t, http.StatusOK, `{"output":{
		"observation":{"season_flag":2,"time_flag":3},
		"sun":{"sun_aveflux":129.5}}}`)

	when := time.GoDate(2024, time.March, 21, 3, 0, 0, 0, time.LocationUTC)

	alm, err := airglow.AlmanacAt(t.Context(), when, airglow.Paranal)
	if err != nil {
		t.Fatalf("AlmanacAt: %v", err)
	}

	if *gotPath != "/api/skycalc_almanac" {
		t.Errorf("service saw %q, want the almanac path", *gotPath)
	}

	if alm.SolarFluxSFU != 129.5 {
		t.Errorf("SolarFluxSFU = %v, want the value the service reported (129.5)", alm.SolarFluxSFU)
	}
}

// A service that answers with something that is not the expected document must
// fail rather than yield a zero almanac — a zero solar flux is a real value
// that every downstream airglow estimate would quietly use.
func TestAlmanacAtRefusesAnUndecodableAnswer(t *testing.T) {
	serveAlmanac(t, http.StatusOK, `not json at all`)

	when := time.GoDate(2024, time.March, 21, 3, 0, 0, 0, time.LocationUTC)

	if _, err := airglow.AlmanacAt(t.Context(), when, airglow.Paranal); err == nil {
		t.Fatal("an undecodable response produced an almanac")
	}
}

// A non-2xx never reaches the decoder: it arrives as a typed error, so an error
// page is never parsed as data.
func TestAlmanacAtReportsAServiceFailure(t *testing.T) {
	serveAlmanac(t, http.StatusServiceUnavailable, `<html>down for maintenance</html>`)

	when := time.GoDate(2024, time.March, 21, 3, 0, 0, 0, time.LocationUTC)

	if _, err := airglow.AlmanacAt(t.Context(), when, airglow.Paranal); err == nil {
		t.Fatal("a 503 produced an almanac")
	}
}
