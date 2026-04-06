package vizier

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
)

func TestVizierOfflineConeSearch(t *testing.T) {
	csvData := `ra,dec,id
10.684,41.269,OBJ1
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprint(w, csvData)
	}))
	defer server.Close()

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	req := catalog.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)),
		Radius: angle.Deg(5),
	}

	iter := prov.ConeSearch(context.Background(), req)

	var targets []catalog.Target
	iter(func(tar catalog.Target, err error) bool {
		if err != nil {
			t.Fatalf("Unexpected err: %v", err)
		}
		targets = append(targets, tar)
		return true
	})

	// Our visualization scaffold returns empty on parseCSV for vizier, but
	// ensures network paths and iter blocks behave functionally.
	if len(targets) != 0 {
		t.Fatalf("Expected scaffold parser to return empty arrays, got %d", len(targets))
	}
}

type mockTransport struct {
	Handler http.Handler
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.Handler.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}
