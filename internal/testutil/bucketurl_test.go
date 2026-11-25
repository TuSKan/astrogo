package testutil_test

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestDirURLNamesTheDirectoryExactly: DirURL gives a file:// URL asking for
// the directory to be created, whose path is the directory's own, a '#' and a
// '%' included, which a URL built by concatenation would cut at or misread.
// FileURL is the same URL.
func TestDirURLNamesTheDirectoryExactly(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "a#b%c")

	got, err := testutil.DirURL(dir)
	if err != nil {
		t.Fatalf("DirURL: %v", err)
	}

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("DirURL gave %q, which does not parse: %v", got, err)
	}

	if u.Scheme != "file" || u.Host != "" || u.Query().Get("create_dir") != "true" {
		t.Errorf("DirURL = %q, want a file:// URL with no host and create_dir=true", got)
	}

	want := filepath.ToSlash(dir)
	if !strings.HasPrefix(want, "/") {
		want = "/" + want
	}

	if u.Path != want {
		t.Errorf("DirURL's path = %q, want %q", u.Path, want)
	}

	if f := testutil.FileURL(t, dir); f != got {
		t.Errorf("FileURL = %q, DirURL = %q; want the same URL", f, got)
	}
}
