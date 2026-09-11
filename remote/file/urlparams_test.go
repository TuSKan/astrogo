package file_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/time"
)

// The portable bucket-URL wrappers, tested where they apply.
//
// These are gocloud's own, handled by blob.OpenBucket before any driver sees
// the URL, so they work on every scheme at once and this package needs no code
// for either. They are tested anyway because remote's endpoint registry
// documents them as the supported way to point an endpoint at a nested mirror
// or one exact object — a claim about a dependency's behaviour is still a
// claim, and this is the layer that would have to change if it stopped being
// true.
//
// They used to be tested through SetURL and GetFile, which reached this
// behaviour through the registry, the consent gate and the download path. Every
// assertion was still on the path the server saw, so the subject was always
// key resolution inside an opened bucket: this package's.

// serveRecording stands up a server answering everything with body, recording
// the last path it was asked for.
func serveRecording(t *testing.T, body string) (baseURL string, lastPath *string) {
	t.Helper()

	var seen string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path

		http.ServeContent(w, r, "object.dat", time.Unix(1_700_000_000, 0), strings.NewReader(body))
	}))

	t.Cleanup(srv.Close)

	return srv.URL, &seen
}

// TestKeyParamServesOneObjectUnderAnyName covers the wrapper an endpoint uses
// when its source publishes one file under a name astrogo would never ask for.
//
// "?key=" makes every key in the bucket resolve to that one object, which is
// what lets a caller keep asking for "finals2000A.all" while the server holds
// "/archive/2026-08/dump.dat".
func TestKeyParamServesOneObjectUnderAnyName(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("EOP row\n", 100)

	base, servedPath := serveRecording(t, body)

	u, err := url.Parse(base + "/archive/2026-08")
	if err != nil {
		t.Fatal(err)
	}

	u.RawQuery = url.Values{"key": {"dump.dat"}}.Encode()

	bucket, err := file.Open(t.Context(), u.String())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, err := bucket.ReadAll(t.Context(), "a-name-the-server-never-heard-of")
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if *servedPath != "/archive/2026-08/dump.dat" {
		t.Errorf("server saw %q, want the single key the URL named", *servedPath)
	}

	if !bytes.Equal(got, []byte(body)) {
		t.Error("content differs from the single object")
	}
}

// TestWithoutKeyParamANameResolvesUnderneath is the counterpart, and the reason
// Endpoint.URL's doc comment insists a KindFile URL is a directory prefix.
//
// Without "?key=", a URL naming one exact object has the caller's name resolved
// underneath it — so the request goes to .../dump.dat/finals2000A.all, which is
// nothing, and the failure is a clean 404 rather than a silent wrong read.
func TestWithoutKeyParamANameResolvesUnderneath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/archive/dump.dat" {
			http.NotFound(w, r)

			return
		}

		http.ServeContent(w, r, "dump.dat", time.Unix(1_700_000_000, 0), strings.NewReader("x"))
	}))
	defer srv.Close()

	bucket, err := file.Open(t.Context(), srv.URL+"/archive/dump.dat")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if _, err := bucket.ReadAll(t.Context(), "finals2000A.all"); err == nil {
		t.Fatal("expected a clean failure when the bucket URL names one exact object")
	}
}

// TestPrefixParamScopesTheBucket covers the other wrapper: an endpoint pointed
// at a mirror that nests the files deeper needs no astrogo change, only a
// longer URL.
func TestPrefixParamScopesTheBucket(t *testing.T) {
	t.Parallel()

	base, servedPath := serveRecording(t, "id,ra,dec\n")

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}

	u.RawQuery = url.Values{"prefix": {"mirror/openngc/"}}.Encode()

	bucket, err := file.Open(t.Context(), u.String())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if _, err := bucket.ReadAll(t.Context(), "NGC.csv"); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if *servedPath != "/mirror/openngc/NGC.csv" {
		t.Errorf("server saw %q, want the prefixed path", *servedPath)
	}
}
