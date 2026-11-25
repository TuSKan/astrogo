package iers

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"
)

// TestFSLoaderReadsTheObject is the success path: the bytes as stored, and the
// object's modification time, which seeds the retry cooldown.
func TestFSLoaderReadsTheObject(t *testing.T) {
	t.Parallel()

	written := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"eop/finals2000A.data": {Data: []byte(sampleFinals2000A), ModTime: written},
	}

	data, err := FSLoader{FS: fsys, Name: "eop/finals2000A.data"}.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}

	if string(data.Raw) != sampleFinals2000A {
		t.Errorf("Cached returned %d bytes, not the bulletin as stored", len(data.Raw))
	}

	if !data.ModTime.Equal(written) {
		t.Errorf("Cached ModTime = %v, want the object's %v", data.ModTime, written)
	}
}

// TestFSLoaderTellsAbsenceFromFailure holds the distinction the path-based
// loader it replaced erased (#509): that one returned ErrNoEOPData for any
// read error, so a bulletin that was there and could not be read looked
// exactly like one that had never been seeded.
func TestFSLoaderTellsAbsenceFromFailure(t *testing.T) {
	t.Parallel()

	t.Run("missing is ErrNoEOPData", func(t *testing.T) {
		t.Parallel()

		_, err := FSLoader{FS: fstest.MapFS{}, Name: "finals2000A.data"}.Cached(t.Context())
		if !errors.Is(err, ErrNoEOPData) {
			t.Errorf("Cached on a missing object = %v, want ErrNoEOPData", err)
		}
	})

	t.Run("unreadable is itself", func(t *testing.T) {
		t.Parallel()

		_, err := FSLoader{FS: deniedFS{}, Name: "finals2000A.data"}.Cached(t.Context())
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("Cached on an unreadable object = %v, want one wrapping fs.ErrPermission", err)
		}

		if errors.Is(err, ErrNoEOPData) {
			t.Errorf("Cached on an unreadable object = %v, which reads as \"nothing seeded\"", err)
		}
	})

	t.Run("zero value is a misconfiguration", func(t *testing.T) {
		t.Parallel()

		// A nil FS would panic inside fs.ReadFile; it is reported instead,
		// and not as absence.
		_, err := FSLoader{}.Cached(t.Context())
		if !errors.Is(err, errNoFS) {
			t.Errorf("Cached on a zero FSLoader = %v, want errNoFS", err)
		}
	})
}

// TestFSLoaderNeverFetches: reading what is already there is not a download,
// so Fetch reports absence rather than reaching anywhere.
func TestFSLoaderNeverFetches(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"finals2000A.data": {Data: []byte(sampleFinals2000A)}}

	if _, err := (FSLoader{FS: fsys, Name: "finals2000A.data"}).Fetch(t.Context()); !errors.Is(err, ErrNoEOPData) {
		t.Errorf("Fetch = %v, want ErrNoEOPData", err)
	}
}

// deniedFS refuses every open with fs.ErrPermission: a bulletin that is there
// and cannot be read.
type deniedFS struct{}

func (deniedFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}
