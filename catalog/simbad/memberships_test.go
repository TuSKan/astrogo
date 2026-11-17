package simbad

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// The membership query reads h_link for exactly the objects asked about, the
// parent named by its main identifier, and asks for distinct rows: h_link
// records a link once per paper, and 17 Tau's membership of the Pleiades is
// six rows otherwise.
func TestBuildMembershipQueryAsksForDistinctParents(t *testing.T) {
	t.Parallel()

	got := buildMembershipQuery([]int64{3396054, 3396297})

	want := "SELECT DISTINCT h_link.child AS oidref, basic.main_id AS parent " +
		"FROM h_link JOIN basic ON basic.oid = h_link.parent WHERE h_link.child IN (3396054,3396297)"
	if got != want {
		t.Errorf("buildMembershipQuery =\n%s\nwant\n%s", got, want)
	}
}

// membershipAnswer answers a membership query with parents(oid) for each
// object it asks about, one row per parent.
func membershipAnswer(t *testing.T, query string, parents func(oid int64) []string) string {
	t.Helper()

	_, list, ok := strings.Cut(query, "IN (")
	if !ok {
		t.Errorf("membership query names no objects: %s", query)
		return noMemberships
	}

	var b strings.Builder

	b.WriteString(noMemberships)

	for f := range strings.SplitSeq(strings.TrimSuffix(list, ")"), ",") {
		oid, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			t.Errorf("membership query asks about %q: %v", f, err)
			continue
		}

		for _, p := range parents(oid) {
			fmt.Fprintf(&b, "%d,%q\n", oid, p)
		}
	}

	return b.String()
}

// TestSearchBrightAddsWhatEachObjectIsAMemberOf: every bright object comes
// with what SIMBAD's hierarchy makes it a member of, asked in the same
// batches as its identifiers, so 2001 objects take three membership requests,
// none asking about more than identifierBatch. An object in a system and a
// cluster is a member of both; one in neither is a member of nothing (#1182).
func TestSearchBrightAddsWhatEachObjectIsAMemberOf(t *testing.T) {
	const rows = 2*identifierBatch + 1

	var (
		requests atomic.Int32
		largest  atomic.Int32
	)

	var b strings.Builder

	b.WriteString("oid,main_id,ra,dec,otype,pmra,pmdec,plx_value,rvz_radvel,vmag\n")

	for i := range rows {
		fmt.Fprintf(&b, "%d,* s%d,10.0,20.0,*,0,0,0,0,%.6f\n", i+1, i+1, float64(i)/rows)
	}

	parents := func(oid int64) []string {
		switch oid % 3 {
		case 0:
			return nil
		case 1:
			return []string{fmt.Sprintf("* sys%d", oid)}
		default:
			return []string{fmt.Sprintf("* sys%d", oid), "Cl Melotte   22"}
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/csv")

		switch q := r.PostForm.Get("QUERY"); {
		case isMembershipQuery(q):
			requests.Add(1)

			_, list, _ := strings.Cut(q, "IN (")
			if n := int32(strings.Count(list, ",") + 1); n > largest.Load() {
				largest.Store(n)
			}

			_, _ = w.Write([]byte(membershipAnswer(t, q, parents)))
		case isIdentifierQuery(q):
			_, _ = w.Write([]byte(identifierAnswer(t, q, func(oid int64) string { return fmt.Sprintf("* s%d", oid) })))
		default:
			_, _ = w.Write([]byte(b.String()))
		}
	}))
	t.Cleanup(server.Close)

	redirect(t, server.URL)

	targets, errs := drainBright(New(), resolve.BrightRequest{MaxVMag: 9})
	if len(errs) != 0 || len(targets) != rows {
		t.Fatalf("got %d targets and errors %v, want all %d and none", len(targets), errs, rows)
	}

	if n := requests.Load(); n != 3 {
		t.Errorf("%d membership requests for %d objects, want 3", n, rows)
	}

	if n := largest.Load(); n > identifierBatch {
		t.Errorf("a membership request asked about %d objects, want at most %d", n, identifierBatch)
	}

	for i, tgt := range targets {
		if want := parents(int64(i + 1)); !slices.Equal(tgt.MemberOf, want) {
			t.Fatalf("object %d is a member of %q, want %q, SIMBAD's spelling kept", i+1, tgt.MemberOf, want)
		}
	}
}

// TestSearchBrightListsObjectsWithoutTheirMemberships: when memberships cannot
// be fetched, the objects still come, followed by an error counting those
// without; a star system's record and its components are then both listed as
// they are. The list is not cached, so the next call asks again.
func TestSearchBrightListsObjectsWithoutTheirMemberships(t *testing.T) {
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
		case isMembershipQuery(q) && strings.Contains(q, fmt.Sprintf("IN (%d,", identifierBatch+1)):
			// The second batch is refused; a 400 is not retried.
			http.Error(w, "refused", http.StatusBadRequest)
		case isMembershipQuery(q):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(membershipAnswer(t, q, func(oid int64) []string { return []string{fmt.Sprintf("* sys%d", oid)} })))
		case isIdentifierQuery(q):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(identifierAnswer(t, q, func(oid int64) string { return fmt.Sprintf("* s%d", oid) })))
		default:
			brightRequests.Add(1)
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(b.String()))
		}
	}))
	t.Cleanup(server.Close)

	redirect(t, server.URL)

	p := New()

	targets, errs := drainBright(p, resolve.BrightRequest{MaxVMag: 9})
	if len(targets) != rows {
		t.Fatalf("got %d targets, want all %d", len(targets), rows)
	}

	if len(errs) != 1 || !errors.Is(errs[0], errMembershipsUnavailable) ||
		!strings.Contains(errs[0].Error(), fmt.Sprintf("for 5 of %d objects", rows)) {
		t.Fatalf("errors = %v, want one wrapping errMembershipsUnavailable that counts the 5 objects without", errs)
	}

	if len(targets[0].MemberOf) != 1 || len(targets[rows-1].MemberOf) != 0 {
		t.Errorf("first object's memberships %q, last's %q: want the first batch's kept and the refused batch's empty",
			targets[0].MemberOf, targets[rows-1].MemberOf)
	}

	before := brightRequests.Load()
	if _, errs := drainBright(p, resolve.BrightRequest{MaxVMag: 9}); len(errs) != 1 || brightRequests.Load() == before {
		t.Errorf("a second call sent %d bright requests and yielded %v; a list without its memberships must not be cached",
			brightRequests.Load()-before, errs)
	}
}

// TestParseMemberships: one row per link, so an object in a system and a
// cluster comes on two rows and is a member of both, in SIMBAD's spelling.
func TestParseMemberships(t *testing.T) {
	t.Parallel()

	got, err := parseMemberships(strings.NewReader(
		"oidref,parent\n3396054,\"* alf Cen\"\n661210,\"Cl Melotte   22\"\n661210,\"* 17 Tau AB\"\n"))
	if err != nil {
		t.Fatalf("parseMemberships: %v", err)
	}

	if want := []string{"* alf Cen"}; !slices.Equal(got[3396054], want) {
		t.Errorf("alf Cen A is a member of %q, want %q", got[3396054], want)
	}

	if want := []string{"Cl Melotte   22", "* 17 Tau AB"}; !slices.Equal(got[661210], want) {
		t.Errorf("17 Tau is a member of %q, want %q", got[661210], want)
	}

	if empty, err := parseMemberships(strings.NewReader(noMemberships)); err != nil || len(empty) != 0 {
		t.Errorf("an answer with no links = %v, %v; want none and no error", empty, err)
	}

	if _, err := parseMemberships(strings.NewReader("oidref,ids\n1,x\n")); !errors.Is(err, ErrMissingColumn) {
		t.Errorf("an answer without a parent column: err = %v, want ErrMissingColumn", err)
	}

	if _, err := parseMemberships(strings.NewReader("oidref,parent\nabc,\"x\"\n")); !errors.Is(err, errRowWithoutObject) {
		t.Errorf("a row naming no object: err = %v, want errRowWithoutObject", err)
	}
}

// TestOneMissingListDoesNotCostTheOther: identifiers and memberships are asked
// in two passes, so when SIMBAD refuses every query for one, the objects still
// carry the other, and when it refuses both, the error says so for each.
func TestOneMissingListDoesNotCostTheOther(t *testing.T) {
	const rows = 3

	for _, c := range []struct {
		name                     string
		refuseIDs, refuseMembers bool
	}{
		{"identifiers refused", true, false},
		{"memberships refused", false, true},
		{"both refused", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
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

				q := r.PostForm.Get("QUERY")

				switch {
				case isMembershipQuery(q) && c.refuseMembers, isIdentifierQuery(q) && c.refuseIDs:
					http.Error(w, "refused", http.StatusBadRequest)
				case isMembershipQuery(q):
					w.Header().Set("Content-Type", "text/csv")
					_, _ = w.Write([]byte(membershipAnswer(t, q, func(oid int64) []string { return []string{fmt.Sprintf("* sys%d", oid)} })))
				case isIdentifierQuery(q):
					w.Header().Set("Content-Type", "text/csv")
					_, _ = w.Write([]byte(identifierAnswer(t, q, func(oid int64) string { return fmt.Sprintf("* s%d", oid) })))
				default:
					w.Header().Set("Content-Type", "text/csv")
					_, _ = w.Write([]byte(b.String()))
				}
			}))
			t.Cleanup(server.Close)

			redirect(t, server.URL)

			targets, errs := drainBright(New(), resolve.BrightRequest{MaxVMag: 9})
			if len(targets) != rows || len(errs) != 1 {
				t.Fatalf("got %d targets and errors %v, want all %d and one error", len(targets), errs, rows)
			}

			if got := errors.Is(errs[0], errIdentifiersUnavailable); got != c.refuseIDs {
				t.Errorf("error %v wraps errIdentifiersUnavailable: %v, want %v", errs[0], got, c.refuseIDs)
			}

			if got := errors.Is(errs[0], errMembershipsUnavailable); got != c.refuseMembers {
				t.Errorf("error %v wraps errMembershipsUnavailable: %v, want %v", errs[0], got, c.refuseMembers)
			}

			for _, tgt := range targets {
				if hasIDs := len(tgt.Aliases) > 0; hasIDs == c.refuseIDs {
					t.Errorf("%s has identifiers %q, want them only when they were not refused", tgt.ID, tgt.Aliases)
				}

				if hasMembers := len(tgt.MemberOf) > 0; hasMembers == c.refuseMembers {
					t.Errorf("%s is a member of %q, want memberships only when they were not refused", tgt.ID, tgt.MemberOf)
				}
			}
		})
	}
}
