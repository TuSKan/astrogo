package testutil

import (
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
)

// FileURL converts an OS directory path — almost always a t.TempDir() — into
// the file:// filesystem URL remote.OpenFS expects, so tests can stand up a
// local fake source or cache without astrogo itself carrying a path-to-URL
// helper in its public API. See [DirURL], which it fails the test on.
func FileURL(tb testing.TB, dir string) string {
	tb.Helper()

	u, err := DirURL(dir)
	if err != nil {
		tb.Fatal(err)
	}

	return u
}

// DirURL is [FileURL] for a caller with no test to fail, a TestMain.
//
// The URL carries create_dir=true, since the file:// opener creates its
// directory only when asked and would otherwise fail on a directory that does
// not exist yet. It is built through url.URL so a temp path containing '#' or a
// stray '%' — which t.TempDir can produce from a test name — encodes
// correctly instead of silently truncating.
func DirURL(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("testutil.DirURL(%q): %w", dir, err)
	}

	slash := filepath.ToSlash(abs)
	if slash == "" || slash[0] != '/' {
		slash = "/" + slash // Windows drive-letter paths are not "/"-rooted
	}

	u := url.URL{Scheme: "file", Path: slash, RawQuery: "create_dir=true"}

	return u.String(), nil
}
