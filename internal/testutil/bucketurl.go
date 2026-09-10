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
//
// It also carries no_tmp_dir=1. fileblob stages every write through a
// temporary file named from the clock, and by default puts that file in
// os.TempDir rather than in the bucket — so two buckets writing the same
// object name collide there even though they share nothing else. On Windows
// the clock does not advance between the writes (measured: one distinct
// UnixNano across 2000 consecutive reads), so the names are identical and one
// writer renames the other's staging file away. Measured, eight writers over
// 40 rounds: 59 failures in 320 with the default, 0 with this (#241).
//
// It is the better default independently of the race. os.TempDir is often on a
// different volume from the cache directory, and a cross-volume rename is a
// full copy — a second write of a multi-gigabyte kernel. fileblob's own doc
// comment raises exactly this.
//
// Writers of one key inside one process are a different case and need a
// different guard, since staging inside the bucket makes them share the key's
// own path as a name; see file.WriteLock.
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

	u := url.URL{Scheme: "file", Path: slash, RawQuery: "create_dir=true&no_tmp_dir=1"}

	return u.String()
}
