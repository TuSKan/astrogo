package simbad

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// The identifier query reads the ids table, one row per object, for exactly
// the objects asked about: an ident join would give one row per identifier,
// rows a limit could cut.
func TestBuildIdentifierQueryReadsTheIdsTable(t *testing.T) {
	t.Parallel()

	got := buildIdentifierQuery([]int64{799866, 3133169})
	if want := "SELECT oidref, ids FROM ids WHERE oidref IN (799866,3133169)"; got != want {
		t.Errorf("buildIdentifierQuery = %q, want %q", got, want)
	}

	if strings.Contains(got, "ident") {
		t.Errorf("the identifier query joins ident, which a row limit can cut: %s", got)
	}
}

// TestSearchBrightAddsEachObjectsIdentifiers: every bright object comes with
// the identifiers SIMBAD lists for it, fetched in batches of identifierBatch
// once the objects are known, so 2001 objects take three identifier requests,
// none asking about more than identifierBatch (#1154). Without them a SIMBAD
// object shares no designation with another catalog's record of it.
func TestSearchBrightAddsEachObjectsIdentifiers(t *testing.T) {
	const rows = 2*identifierBatch + 1

	var (
		idRequests atomic.Int32
		largest    atomic.Int32
	)

	var b strings.Builder

	b.WriteString("oid,main_id,ra,dec,otype,pmra,pmdec,plx_value,rvz_radvel,vmag\n")

	for i := range rows {
		fmt.Fprintf(&b, "%d,* s%d,10.0,20.0,*,0,0,0,0,%.6f\n", i+1, i+1, float64(i)/rows)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/csv")

		q := r.PostForm.Get("QUERY")
		if isMembershipQuery(q) {
			_, _ = w.Write([]byte(noMemberships))
			return
		}

		if !isIdentifierQuery(q) {
			_, _ = w.Write([]byte(b.String()))
			return
		}

		idRequests.Add(1)

		_, list, _ := strings.Cut(q, "IN (")
		if n := int32(strings.Count(list, ",") + 1); n > largest.Load() {
			largest.Store(n)
		}

		_, _ = w.Write([]byte(identifierAnswer(t, q, func(oid int64) string { return fmt.Sprintf("* s%d|HD  %d", oid, oid) })))
	}))
	t.Cleanup(server.Close)

	redirect(t, server.URL)

	targets, errs := drainBright(New(), resolve.BrightRequest{MaxVMag: 9})
	if len(errs) != 0 || len(targets) != rows {
		t.Fatalf("got %d targets and errors %v, want all %d and none", len(targets), errs, rows)
	}

	if n := idRequests.Load(); n != 3 {
		t.Errorf("%d identifier requests for %d objects, want 3", n, rows)
	}

	if n := largest.Load(); n > identifierBatch {
		t.Errorf("an identifier request asked about %d objects, want at most %d", n, identifierBatch)
	}

	for i, tgt := range targets {
		want := []string{fmt.Sprintf("* s%d", i+1), fmt.Sprintf("HD  %d", i+1)}
		if !slices.Equal(tgt.Aliases, want) {
			t.Fatalf("object %d's aliases = %q, want %q, SIMBAD's spelling kept", i+1, tgt.Aliases, want)
		}
	}
}

// TestSearchBrightListsObjectsWithoutTheirIdentifiers: when identifiers
// cannot be fetched, the objects still come, those without identifiers
// followed by an error counting them; nothing in the sky is dropped for want
// of an alias. The list is not cached, so the next call asks again.
func TestSearchBrightListsObjectsWithoutTheirIdentifiers(t *testing.T) {
	const rows = identifierBatch + 5

	var b strings.Builder

	b.WriteString("oid,main_id,ra,dec,otype,pmra,pmdec,plx_value,rvz_radvel,vmag\n")

	for i := range rows {
		fmt.Fprintf(&b, "%d,* s%d,10.0,20.0,*,0,0,0,0,%.6f\n", i+1, i+1, float64(i)/rows)
	}

	var brightRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		q := r.PostForm.Get("QUERY")

		switch {
		case isMembershipQuery(q):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(noMemberships))
		case !isIdentifierQuery(q):
			brightRequests.Add(1)
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(b.String()))
		case strings.Contains(q, fmt.Sprintf("IN (%d,", identifierBatch+1)):
			// The second batch is refused; a 400 is not retried.
			http.Error(w, "refused", http.StatusBadRequest)
		default:
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(identifierAnswer(t, q, func(oid int64) string { return fmt.Sprintf("* s%d", oid) })))
		}
	}))
	t.Cleanup(server.Close)

	redirect(t, server.URL)

	p := New()

	targets, errs := drainBright(p, resolve.BrightRequest{MaxVMag: 9})
	if len(targets) != rows {
		t.Fatalf("got %d targets, want all %d", len(targets), rows)
	}

	if len(errs) != 1 || !errors.Is(errs[0], errIdentifiersUnavailable) ||
		!strings.Contains(errs[0].Error(), fmt.Sprintf("for 5 of %d objects", rows)) {
		t.Fatalf("errors = %v, want one wrapping errIdentifiersUnavailable that counts the 5 objects without identifiers", errs)
	}

	if len(targets[0].Aliases) != 1 || len(targets[rows-1].Aliases) != 0 {
		t.Errorf("first object's aliases %q, last's %q: want the first batch's kept and the refused batch's empty",
			targets[0].Aliases, targets[rows-1].Aliases)
	}

	before := brightRequests.Load()
	if _, errs := drainBright(p, resolve.BrightRequest{MaxVMag: 9}); len(errs) != 1 || brightRequests.Load() == before {
		t.Errorf("a second call sent %d bright requests and yielded %v; a list without its identifiers must not be cached",
			brightRequests.Load()-before, errs)
	}
}

func TestParseIdentifiers(t *testing.T) {
	t.Parallel()

	got, err := parseIdentifiers(strings.NewReader(
		"oidref,ids\n799866,\"* eps Ori|HD  37128|NAME Alnilam\"\n12,\"\"\n"))
	if err != nil {
		t.Fatalf("parseIdentifiers: %v", err)
	}

	if want := []string{"* eps Ori", "HD  37128", "NAME Alnilam"}; !slices.Equal(got[799866], want) {
		t.Errorf("eps Ori's identifiers = %q, want %q, SIMBAD's padding kept", got[799866], want)
	}

	if ids, ok := got[12]; !ok || len(ids) != 0 {
		t.Errorf("an object with no identifiers = %q (listed %v), want listed with none", ids, ok)
	}

	if _, err := parseIdentifiers(strings.NewReader("oid,main_id\n1,x\n")); !errors.Is(err, ErrMissingColumn) {
		t.Errorf("an answer without oidref and ids: err = %v, want ErrMissingColumn", err)
	}

	if _, err := parseIdentifiers(strings.NewReader("oidref,ids\nabc,\"x\"\n")); !errors.Is(err, errRowWithoutObject) {
		t.Errorf("a row naming no object: err = %v, want errRowWithoutObject", err)
	}
}
