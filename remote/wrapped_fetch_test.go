package remote

import (
	"bytes"
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

// TestGetFileThroughQueryWrappers is #487 end to end: a source reached through
// ?key= or ?prefix=, or a data dir scoped with ?prefix=, could not download.
// The source lost ContextFS, which a download requires, and the data dir lost
// CreateExclFS, which the download lock requires; every wrapped configuration
// failed while the plain one worked.
func TestGetFileThroughQueryWrappers(t *testing.T) {
	t.Parallel()

	withQuery := func(u, q string) string {
		if strings.Contains(u, "?") {
			return u + "&" + q
		}

		return u + "?" + q
	}

	src := testutil.FileURL(t, t.TempDir())

	srcFS, err := file.OpenFS(src)
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	if err := WriteFile(context.Background(), srcFS, "planets/de440s.bsp", bytes.NewReader([]byte("kernel"))); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	for _, c := range []struct {
		name, srcURL, dataDirQuery, get string
	}{
		{"plain", src, "", "planets/de440s.bsp"},
		{"source ?key=", withQuery(src, "key=planets/de440s.bsp"), "", "de440s.bsp"},
		{"source ?prefix=", withQuery(src, "prefix=planets/"), "", "de440s.bsp"},
		{"data dir ?prefix=", src, "prefix=astrogo/", "planets/de440s.bsp"},
	} {
		cl := NewClient()

		dataDir := testutil.FileURL(t, t.TempDir())
		if c.dataDirQuery != "" {
			dataDir = withQuery(dataDir, c.dataDirQuery)
		}

		cl.SetDataDir(dataDir)

		if err := cl.SetURL(NAIFSPK, c.srcURL); err != nil {
			t.Fatalf("%s: SetURL: %v", c.name, err)
		}

		cl.EnableDownloads(0, NAIFSPK)

		fsys, key, err := cl.GetFile(context.Background(), NAIFSPK, c.get)
		if err != nil {
			t.Errorf("%s: GetFile: %v", c.name, err)

			continue
		}

		if got, err := fs.ReadFile(fsys, key); err != nil || string(got) != "kernel" {
			t.Errorf("%s: the cached object reads %q, %v", c.name, got, err)
		}
	}
}
