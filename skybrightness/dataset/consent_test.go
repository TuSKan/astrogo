package dataset_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/internal/remotetest"
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

// A refused download says what was refused: the preset, how much reference
// data it needs, and that the download policy refused it.
//
// # Why this needs a test at all
//
// Because it is a designed behavior rather than an incidental error string.
// A caller whose own policy caps the size needs the number to raise the cap
// to, and until a test existed nothing had ever executed the message.
//
// # Why it needs no network
//
// The policy is asked about downloads, not every request: Inputs resolves
// the passband from an API first, and only then reaches the star map, which
// is the first download. This test used to be network-tagged for the
// passband alone, so it ran only nightly and skipped whenever SVO was down. A
// fake SVO answers that request here, and the star map's endpoint points at a
// server that counts what reaches it, so the refusal is also shown to be made
// without asking the source.
func TestOpenNamesWhatThePolicyRefused(t *testing.T) {
	// Not parallel: the endpoint overrides, the policy and the data
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
		http.Error(w, "this test refuses every download, so nothing should ask", http.StatusTeapot)
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

	// A cache of this test's own. With a warm one Open succeeds under any
	// policy, because an immutable object is reused on existence
	// alone — correct, and enough to make the first version of this test pass
	// on a cold machine and fail on the one that had just run the example.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	remotetest.RefuseDownloads(t)

	_, err := dataset.Open(context.Background(), dataset.Spec{
		Preset: skybrightness.GAMBONSWeb,
	})
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("got %v, want ErrDownloadDenied", err)
	}

	for _, want := range []string{"gambons-web", "MB", "download policy"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q, so a caller is not told what "+
				"was refused:\n%v", want, err)
		}
	}

	if n := starMapHits.Load(); n != 0 {
		t.Errorf("the star map's source was asked %d times under a refusing policy", n)
	}
}
