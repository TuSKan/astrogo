package sbdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/provider"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestSBDBResolver(t *testing.T) {
	jsonData := `{
		"object": {
			"spkid": "20002062",
			"fullname": "2062 Aten (1976 AA)",
			"des": "2062",
			"kind": "a"
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, jsonData)
	}))
	defer server.Close()

	originalURL := sbdbQueryAPI
	sbdbQueryAPI = server.URL
	defer func() { sbdbQueryAPI = originalURL }()

	prov := New()

	tar, ok := prov.Resolve("aten")
	if !ok {
		t.Fatalf("Failed to resolve Aten")
	}
	testutil.AssertEqual(t, "Resolve ID name", tar.Name, "2062 Aten (1976 AA)")
	testutil.AssertEqual(t, "Resolve SPKID", tar.SPKID, "20002062")

	// Test cache bypassing HTTP mock and testing async SeqIterator
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := provider.ObjectRequest{Query: "aten"}
	iter := prov.ResolveObject(ctx, req)

	var targets []provider.Target
	iter(func(tar provider.Target, err error) bool {
		if err == nil {
			targets = append(targets, tar)
		}
		return true
	})

	if len(targets) != 1 {
		t.Fatalf("Expected 1 targets from stream, got %d", len(targets))
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "sbdb" {
		t.Errorf("expected sbdb, got %s", p.Name())
	}
	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != provider.CapObjectResolution {
		t.Errorf("expected CapObjectResolution, got %v", caps)
	}

	_, _ = p.Resolve("non_existent_body")
	_ = p.Search("non_existent_body")
}
