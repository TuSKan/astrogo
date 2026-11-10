package remote

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

// A refused copy that cannot be removed is reported with both failures, and
// nothing is fetched to replace it: GetFile would only be handed the same
// unremovable object again. A non-empty directory at the cache key is such an
// object on every backend that keeps directories, and GetFile, asking only
// whether something exists there, serves it like a cached file.
func TestWithReadReportsACopyItCannotRemove(t *testing.T) {
	cleanRemoteState(t)

	src := serveVersioned(t, NAIFSPK, goodCopy, "v1")
	cache, key := cacheKeyFor(t, NAIFSPK)
	put := func(name string) {
		if err := WriteFile(context.Background(), cache, name, strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}

	put(key + "/inside")

	var r reading

	_, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read))
	if err == nil {
		t.Fatal("GetFile over a directory at the cache key: no error")
	}

	if !strings.Contains(err.Error(), "remove") {
		t.Errorf("err %q does not report the failed removal", err)
	}

	if info, statErr := fs.Stat(cache, key); statErr != nil || !info.IsDir() {
		t.Errorf("the directory at the cache key is gone or changed (Stat: %v)", statErr)
	}

	if n := src.downloads(); n != 0 {
		t.Errorf("%d downloads, want 0", n)
	}
}

// GetFile's refusals that come before any cache or source is touched, each
// with its documented sentinel.
func TestGetFileRefusesWhatItCannotFetch(t *testing.T) {
	cleanRemoteState(t)

	for _, c := range []struct {
		name     string
		id       EndpointID
		file     string
		dataDir  string
		sentinel error
	}{
		{"unregistered endpoint", "no.such.endpoint", readName, "", ErrUnknownEndpoint},
		{"API endpoint", JPLHorizons, readName, "", ErrNotFileEndpoint},
		{"no name", NAIFSPK, "", "", ErrCacheNameRequired},
		{"data directory with no backend", NAIFSPK, readName, "nope://cache/", ErrUnknownScheme},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.dataDir != "" {
				before := DataDirURL()

				t.Cleanup(func() { SetDataDir(before) })

				SetDataDir(c.dataDir)
			}

			EnableDownloads(0, NAIFSPK)

			if _, _, err := GetFile(context.Background(), c.id, c.file); !errors.Is(err, c.sentinel) {
				t.Errorf("GetFile(%q, %q): err %v, want %v", c.id, c.file, err, c.sentinel)
			}
		})
	}
}
