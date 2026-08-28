package remote

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// A KindFile endpoint's URL is a bucket root, and the caller's name
// resolves within it. A URL naming one exact object therefore cannot
// serve a name — the failure mode is a 404, not a silent wrong read.
//
// gocloud's portable "key" query parameter is the supported way to point
// an endpoint at one exact object anyway: it wraps the bucket so every key
// resolves to that object. It is handled by blob.OpenBucket itself, before
// any driver sees the URL, so remote needs no code for it and it works on
// every scheme at once.
func TestSetURLToSingleObjectViaKeyParam(t *testing.T) {
	cleanRemoteState(t)

	body := []byte(strings.Repeat("EOP row\n", 100))

	var servedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		servedPath = r.URL.Path

		http.ServeContent(w, r, "whatever.dat", time.Unix(1_700_000_000, 0), bytes.NewReader(body))
	}))
	defer srv.Close()

	// The real object lives at /archive/2026-08/dump.dat, which matches no
	// name astrogo would ask for. "key" makes it answer anyway.
	u, err := url.Parse(srv.URL + "/archive/2026-08")
	if err != nil {
		t.Fatal(err)
	}

	u.RawQuery = url.Values{"key": {"dump.dat"}}.Encode()

	if err := SetURL(IERSFinals2000A, u.String()); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, IERSFinals2000A)

	bucket, key, err := GetFile(context.Background(), IERSFinals2000A, "finals2000A.all",
		WithCacheName("finals2000A.data"))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if servedPath != "/archive/2026-08/dump.dat" {
		t.Errorf("server saw %q, want the single key the URL named", servedPath)
	}

	got, err := bucket.ReadAll(context.Background(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, body) {
		t.Error("cached content differs from the single object")
	}
}

// The counterpart: without "key", a URL naming one exact object resolves
// name underneath it and fails cleanly. Asserted so the guidance in
// Endpoint.URL's doc comment stays true.
func TestSetURLToSingleObjectWithoutKeyParamFails(t *testing.T) {
	cleanRemoteState(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/archive/dump.dat" {
			http.NotFound(w, r)

			return
		}

		http.ServeContent(w, r, "dump.dat", time.Unix(1_700_000_000, 0), strings.NewReader("x"))
	}))
	defer srv.Close()

	if err := SetURL(IERSFinals2000A, srv.URL+"/archive/dump.dat"); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, IERSFinals2000A)

	if _, _, err := GetFile(context.Background(), IERSFinals2000A, "finals2000A.all"); err == nil {
		t.Fatal("expected a clean failure when the endpoint URL names one exact object")
	}
}

// The other portable wrapper: "prefix" scopes a bucket to a subdirectory,
// so an endpoint can point into a mirror that nests the files deeper
// without any astrogo change.
func TestSetURLWithPrefixParam(t *testing.T) {
	cleanRemoteState(t)

	var servedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		servedPath = r.URL.Path

		http.ServeContent(w, r, "NGC.csv", time.Unix(1_700_000_000, 0), strings.NewReader("id,ra,dec\n"))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	u.RawQuery = url.Values{"prefix": {"mirror/openngc/"}}.Encode()

	if err := SetURL(OpenNGC, u.String()); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, OpenNGC)

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if servedPath != "/mirror/openngc/NGC.csv" {
		t.Errorf("server saw %q, want the prefixed path", servedPath)
	}
}
