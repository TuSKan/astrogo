package sbdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// serveQuery answers every SBDB bulk query with body.
func serveQuery(t *testing.T, body string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.JPLSBDBQuery, server.URL); err != nil {
		t.Fatal(err)
	}
}

func searchBright(t *testing.T) ([]resolve.Target, []error) {
	t.Helper()

	var (
		targets []resolve.Target
		errs    []error
	)

	New().SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 12, Limit: 50})(
		func(tgt resolve.Target, err error) bool {
			if err != nil {
				errs = append(errs, err)
			} else {
				targets = append(targets, tgt)
			}

			return true
		})

	return targets, errs
}

// TestSearchBrightRefusesAnAnswerWithoutItsIdentityColumns: every row's name
// and SPK ID come from the full_name and spkid columns the query asks for. An
// answer missing either used to read column 0 in their place, so the body
// below came back named after its own H cell (#1058).
func TestSearchBrightRefusesAnAnswerWithoutItsIdentityColumns(t *testing.T) {
	serveQuery(t, `{"fields":["H"],"data":[["NaN"]]}`)

	targets, errs := searchBright(t)
	if len(targets) != 0 {
		t.Errorf("SearchBright returned %d targets from an answer that names no body: %+v", len(targets), targets)
	}

	if len(errs) == 0 || !errors.Is(errs[0], ErrAPIError) {
		t.Errorf("SearchBright reported %v, want an error wrapping ErrAPIError", errs)
	}
}

// TestSearchBrightReadsAShortRowAsItsCells: a row that stops before the last
// column keeps what it has and leaves the rest unset. It used to index past
// its end and panic (#1058). A comet answer without a K1 column leaves K1
// unset rather than reading it from the name, as the missing column's index
// of 0 used to.
func TestSearchBrightReadsAShortRowAsItsCells(t *testing.T) {
	serveQuery(t, `{"fields":["full_name","spkid","H","G","class"],"data":[["  1 Ceres","20000001"]]}`)

	targets, errs := searchBright(t)
	if len(errs) != 0 {
		t.Fatalf("SearchBright reported %v, want none", errs)
	}

	// The same answer serves both the asteroid and the comet query.
	if len(targets) == 0 {
		t.Fatal("SearchBright returned nothing for a row that names a body")
	}

	for _, tgt := range targets {
		if tgt.Name != "1 Ceres" || tgt.SPKID != "20000001" {
			t.Errorf("target %q (SPK %q), want 1 Ceres (20000001)", tgt.Name, tgt.SPKID)
		}

		if tgt.HasH || tgt.HasM1 || tgt.K1 != 0 {
			t.Errorf("%s: H %v (%v), M1 %v (%v), K1 %v from cells the row does not have",
				tgt.Name, tgt.H, tgt.HasH, tgt.M1, tgt.HasM1, tgt.K1)
		}
	}
}
