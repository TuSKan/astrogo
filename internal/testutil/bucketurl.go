package testutil

import (
	"net/url"
	"path/filepath"
	"testing"
)

// FileURL converts an OS directory path — almost always a t.TempDir() — into
// the "file://" bucket URL remote/file.Open expects, so tests can stand up a
// local fake source or cache without astrogo itself carrying a path-to-URL
// helper in its public API.
//
// The URL carries create_dir=true, since fileblob's URL opener defaults
// CreateDir to false and would otherwise fail on a directory that does not
// exist yet. It is built through url.URL so a temp path containing '#' or a
// stray '%' — which t.TempDir can produce from a test name — encodes
// correctly instead of silently truncating.
func FileURL(tb testing.TB, dir string) string {
	tb.Helper()

	abs, err := filepath.Abs(dir)
	if err != nil {
		tb.Fatalf("testutil.FileURL(%q): %v", dir, err)
	}

	slash := filepath.ToSlash(abs)
	if slash == "" || slash[0] != '/' {
		slash = "/" + slash // Windows drive-letter paths are not "/"-rooted
	}

	u := url.URL{Scheme: "file", Path: slash, RawQuery: "create_dir=true"}

	return u.String()
}
