package time_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/TuSKan/astrogo/remote"
	atime "github.com/TuSKan/astrogo/time"
)

const sampleFinals2000AForGateway = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

func TestParseFinals2000AGateway(t *testing.T) {
	table, err := atime.ParseFinals2000A(strings.NewReader(sampleFinals2000AForGateway))
	if err != nil {
		t.Fatalf("ParseFinals2000A: %v", err)
	}

	if _, err := table.EOP(41684); err != nil {
		t.Errorf("expected MJD 41684 to be covered, got: %v", err)
	}
}

func TestLoadFSGateway(t *testing.T) {
	t.Cleanup(func() { atime.RegisterModel(atime.ZeroModel{}) })

	fsys := fstest.MapFS{
		"finals2000A.all": {Data: []byte(sampleFinals2000AForGateway)},
	}

	if err := atime.LoadFS(fsys, "finals2000A.all"); err != nil {
		t.Fatalf("LoadFS: %v", err)
	}

	lo, hi, ok := atime.Coverage()
	if !ok {
		t.Fatal("expected a coverage-reporting model after LoadFS")
	}

	if lo != 41684 || hi != 41685 {
		t.Errorf("Coverage = [%v, %v], want [41684, 41685]", lo, hi)
	}

	if _, ok := atime.GetModel().(*atime.Table); !ok {
		t.Errorf("expected *Table after LoadFS, got %T", atime.GetModel())
	}
}

func TestFetchAndFetchIfStaleGateway(t *testing.T) {
	t.Cleanup(func() {
		atime.RegisterModel(atime.ZeroModel{})
		remote.Reset()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleFinals2000AForGateway))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	if err := atime.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if _, _, ok := atime.Coverage(); !ok {
		t.Error("expected a coverage-reporting model after Fetch")
	}

	// The registered Table already covers this epoch, so FetchIfStale
	// must short-circuit without error.
	tm := atime.FromJD(2441684.5, atime.UTC) // MJD 41684
	if err := atime.FetchIfStale(context.Background(), tm); err != nil {
		t.Fatalf("FetchIfStale: %v", err)
	}
}

func TestSetRetryCooldownGateway(_ *testing.T) {
	// Exercises the gateway wrapper only; time/internal/iers's own tests
	// cover the throttling behavior itself.
	atime.SetRetryCooldown(0)
	atime.SetRetryCooldown(5 * atime.Minute) // restore the default
}
