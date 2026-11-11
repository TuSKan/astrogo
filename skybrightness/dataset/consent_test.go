package dataset_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
)

// svoProfile is a Johnson V profile in the shape the Spanish Virtual
// Observatory serves: the parameters, then the curve in Angstrom.
const svoProfile = `<?xml version="1.0"?><VOTABLE version="1.1"><RESOURCE type="results">` +
	`<PARAM name="filterID" value="Generic/Bessell.V"/>` +
	`<PARAM name="WavelengthUnit" value="Angstrom"/>` +
	`<PARAM name="DetectorType" value="0"/>` +
	`<PARAM name="MagSys" value="Vega"/>` +
	`<PARAM name="ZeroPoint" value="3630.2172842325"/>` +
	`<PARAM name="ZeroPointUnit" value="Jy"/>` +
	`<TABLE utype="photdm:PhotometryFilter.transmissionCurve.spectrum">` +
	`<FIELD name="Wavelength" unit="Angstrom" datatype="double"/>` +
	`<FIELD name="Transmission" datatype="double"/><DATA><TABLEDATA>` +
	`<TR><TD>4700</TD><TD>0</TD></TR><TR><TD>5000</TD><TD>0.458</TD></TR>` +
	`<TR><TD>5300</TD><TD>1</TD></TR><TR><TD>5600</TD><TD>0.79</TD></TR>` +
	`<TR><TD>6000</TD><TD>0.2</TD></TR><TR><TD>6900</TD><TD>0</TD></TR>` +
	`</TABLEDATA></DATA></TABLE></RESOURCE></VOTABLE>`

// A refused download says which call grants it.
//
// # Why this needs a test at all
//
// Because it is a designed behavior rather than an incidental error string.
// Consent deliberately stays outside this package — a convenience that
// granted its own would fetch 145 MB because somebody typed a preset name —
// and the price of that decision is that a first-time caller hits a refusal.
// The message paying that price is the whole mitigation, and until this test
// existed nothing had ever executed it.
//
// # Why it needs no network
//
// Consent gates downloads, not every request: Inputs resolves the passband
// from an API first, and only then reaches the star map, which is the first
// thing consent applies to. This test used to be network-tagged for the
// passband alone, so it ran only nightly and skipped whenever SVO was down. A
// fake SVO answers that request here, and the star map's endpoint points at a
// server that counts what reaches it, so the refusal is also shown to be made
// without asking the source.
func TestOpenNamesTheConsentItNeeds(t *testing.T) {
	// Not parallel: the endpoint overrides, the consent and the data
	// directory are process-global. Captured first, so the tests after this
	// one do not inherit them (#971).
	t.Cleanup(remote.Capture().Restore)

	svo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(svoProfile))
	}))
	t.Cleanup(svo.Close)

	var starMapHits atomic.Int32

	starMap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		starMapHits.Add(1)
		http.Error(w, "this test grants no consent, so nothing should ask", http.StatusTeapot)
	}))
	t.Cleanup(starMap.Close)

	for id, u := range map[remote.EndpointID]string{
		remote.SVOFilterProfile: svo.URL,
		remote.GaiaStarMap:      starMap.URL + "/",
	} {
		if err := remote.SetURL(id, u); err != nil {
			t.Fatalf("SetURL(%s): %v", id, err)
		}
	}

	// A cache of this test's own. With a warm one Open succeeds with no
	// consent at all, because an immutable object is reused on existence
	// alone — correct, and enough to make the first version of this test pass
	// on a cold machine and fail on the one that had just run the example.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	remote.DisableDownloads()

	_, err := dataset.Open(context.Background(), dataset.Spec{
		Preset: skybrightness.GAMBONSWeb,
	})
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("got %v, want ErrDownloadDenied", err)
	}

	for _, want := range []string{"EnableDownloads", "dataset.Endpoints", "gambons-web"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q, so a caller is told they cannot "+
				"proceed without being told how:\n%v", want, err)
		}
	}

	if n := starMapHits.Load(); n != 0 {
		t.Errorf("the star map's source was asked %d times with no consent granted", n)
	}
}
