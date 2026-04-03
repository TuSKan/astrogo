package catalog

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestSmallBodyProvider(t *testing.T) {
	// Mock the JPL API HTTP Server with valid CSV response
	csvData := `full_name,pdes,name,spkid,kind
2062 Aten,2062,Aten,2002062,Asteroid
1 Halley,1P,Halley,1000036,Comet
MissingName,1234,,2001234,Asteroid
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validates query param population
		kind := r.URL.Query().Get("sb-kind")
		if kind != "a" && kind != "" {
			t.Errorf("Expected kind='a' or '', got %q", kind)
		}
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprint(w, csvData)
	}))
	defer server.Close()

	// Override the package-level URL variable for testing
	originalURL := sbdbQueryAPI
	sbdbQueryAPI = server.URL
	defer func() { sbdbQueryAPI = originalURL }()

	prov, err := NewSmallBodyProvider(SBDBQuery{
		Kind: "a",
	})
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Provider Name", prov.Name(), "sbdb")

	// Test Resolve By ID
	tar, ok := prov.Resolve("2002062")
	if !ok {
		t.Fatalf("Failed to resolve SPKID 2002062")
	}
	testutil.AssertEqual(t, "Resolve ID name", tar.Name, "2062 Aten")

	// Test Resolve By Name
	tar, ok = prov.Resolve("aten")
	if !ok {
		t.Fatalf("Failed to resolve Aten by name")
	}
	testutil.AssertEqual(t, "Resolve name", tar.SPKID, "2002062")

	// Test Search
	results := prov.Search("Halley")
	testutil.AssertEqual(t, "Search count", len(results), 1)
	testutil.AssertEqual(t, "Search match", results[0].SPKID, "1000036")

	// Empty search
	testutil.AssertEqual(t, "Empty search", len(prov.Search("")), 0)

	// Missing properties resolution
	tar, ok = prov.Resolve("1234")
	if !ok {
		t.Fatalf("Failed to resolve target without name")
	}
	testutil.AssertEqual(t, "MissingName resolution", tar.Name, "MissingName")
}
