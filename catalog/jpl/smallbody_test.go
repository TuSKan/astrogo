package jpl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

// serveByCommand points Horizons at a server answering each request with the
// fixture its COMMAND names in fixtures, and returns a Provider using it and
// the commands asked, in order. A command with no fixture is a test failure.
func serveByCommand(t *testing.T, fixtures map[string]string) (*Provider, func() []string) {
	t.Helper()

	var (
		mu    sync.Mutex
		asked []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		command := r.URL.Query().Get("COMMAND")

		mu.Lock()

		asked = append(asked, command)

		mu.Unlock()

		name, ok := fixtures[command]
		if !ok {
			t.Errorf("unexpected Horizons COMMAND %s", command)
			http.Error(w, "no fixture", http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, jsonResultPayload(t, readFixture(t, name))) //nolint:errcheck // test server; a write failure surfaces through the assertions
	}))
	t.Cleanup(server.Close)

	redirect(t, server.URL)

	return New(), func() []string {
		mu.Lock()
		defer mu.Unlock()

		return slices.Clone(asked)
	}
}

// TestSearchAsksSmallBodiesWhenNoMajorBodyIsNamed: Horizons searches its
// major bodies first, matches a name anywhere inside another, and looks no
// further once one matches, so "Eros" came back as Pluto's moon Kerberos and
// "Hebe" as Jupiter's Thebe (#618). When no target carries the query as a
// whole word, Search asks again with a trailing semicolon, small bodies only.
// The fixtures are Horizons' live answers, fetched 2026-10-08.
func TestSearchAsksSmallBodiesWhenNoMajorBodyIsNamed(t *testing.T) {
	for _, c := range []struct {
		query    string
		fixtures map[string]string
		wantName string
		asked    []string
	}{
		{
			query:    "Eros",
			fixtures: map[string]string{"'Eros'": "eros.txt", "'Eros;'": "eros-small.txt"},
			wantName: "433 Eros", asked: []string{"'Eros'", "'Eros;'"},
		},
		{
			query:    "Hebe",
			fixtures: map[string]string{"'Hebe'": "hebe.txt", "'Hebe;'": "hebe-small.txt"},
			wantName: "6 Hebe", asked: []string{"'Hebe'", "'Hebe;'"},
		},
		{
			// A table of OSIRIS-REx and its sample capsule: "Iris" is inside
			// both names, a word of neither.
			query:    "Iris",
			fixtures: map[string]string{"'Iris'": "iris.txt", "'Iris;'": "iris-small.txt"},
			wantName: "7 Iris", asked: []string{"'Iris'", "'Iris;'"},
		},
		{
			// Juno, the spacecraft, carries the word: a fair answer, kept.
			query:    "Juno",
			fixtures: map[string]string{"'Juno'": "juno.txt"},
			wantName: "Juno (spacecraft)", asked: []string{"'Juno'"},
		},
		{
			// A prefix: no small body is named "Tethy", so Tethys stands.
			query:    "Tethy",
			fixtures: map[string]string{"'Tethy'": "tethy.txt", "'Tethy;'": "tethy-small.txt"},
			wantName: "Tethys", asked: []string{"'Tethy'", "'Tethy;'"},
		},
	} {
		t.Run(c.query, func(t *testing.T) {
			p, asked := serveByCommand(t, c.fixtures)

			got, err := p.Resolve(context.Background(), c.query)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", c.query, err)
			}

			if got.Name != c.wantName {
				t.Errorf("Resolve(%q) = %q, want %q", c.query, got.Name, c.wantName)
			}

			if a := asked(); !slices.Equal(a, c.asked) {
				t.Errorf("asked Horizons %q, want %q", a, c.asked)
			}
		})
	}
}

// TestContainsWord pins the whole-word rule the retry turns on.
func TestContainsWord(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		s, word string
		want    bool
	}{
		{"433 Eros", "Eros", true},
		{"Kerberos", "Eros", false},
		{"Thebe", "Hebe", false},
		{"OSIRIS-REx (spacecraft)", "Iris", false},
		{"Juno (spacecraft)", "juno", true},
		{"Juno Centaur Stage (spacecraft)", "Juno", true},
		{"C/2020 F3 (NEOWISE)", "NEOWISE", true},
		{"Erosion", "Eros", false},
		{"anything", "", false},
	} {
		if got := containsWord(c.s, c.word); got != c.want {
			t.Errorf("containsWord(%q, %q) = %v, want %v", c.s, c.word, got, c.want)
		}
	}
}
