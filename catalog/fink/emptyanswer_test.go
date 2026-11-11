package fink

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestResolvePassesOverAnAnswerThatNamesNoBody: a single-object answer with no
// name and no number answers nothing, so Resolve goes on to the bulk table. It
// used to return that answer as a target with no ID and no error, and never
// asked the table (#1061).
func TestResolvePassesOverAnAnswerThatNamesNoBody(t *testing.T) {
	t.Cleanup(remote.Reset)

	table := ssoftParquet(t, []ssoftRow{{name: "Benoitcarry", number: 8467, h1: 15.0, status: 1}})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer

		_, _ = buf.ReadFrom(r.Body)

		if bytes.Contains(buf.Bytes(), []byte("parquet")) {
			_, _ = w.Write(table)

			return
		}

		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
		t.Fatal(err)
	}

	tgt, err := New().Resolve(context.Background(), "8467")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if tgt.ID != "8467" || tgt.Name != "Benoitcarry" {
		t.Errorf("Resolve(8467) = %q (ID %q), want Benoitcarry (8467) from the bulk table", tgt.Name, tgt.ID)
	}
}
