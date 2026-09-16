package file_test

import (
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

// errValidationFailed stands for a validator rejecting staged content.
var errValidationFailed = errors.New("staged content is not what was expected")

// The resume half of the download path, tested here rather than only through
// remote.GetFile.
//
// Reaching it through GetFile means a failure arrives as "the fetch went wrong"
// with the registry, the consent gate and a live source between the assertion
// and the defect. These functions take a filesystem and a key and nothing else, so
// they can be asked directly what they do — and the questions worth asking are
// about a partial nobody wrote, a partial that no longer matches its source,
// and content that fails validation after it was already staged.

// newFS opens a fresh temp directory as a filesystem.
func newFS(t *testing.T) fs.FS {
	t.Helper()

	fsys, err := file.OpenFS(testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	return fsys
}

// seedPartial writes a partial download and the ETag sidecar recording what
// source it came from, which is what ResumePoint reads back.
//
// This replaces file.SavePartial, which recorded the ETag as gocloud object
// metadata. There is no metadata on an io/fs filesystem, so the ETag is a
// sidecar object — and writing one in a test is two ordinary writes rather than
// a function that existed only to carry one string.
func seedPartial(t *testing.T, fsys fs.FS, key, body, etag string) {
	t.Helper()

	if err := file.WriteFile(t.Context(), fsys, key, strings.NewReader(body)); err != nil {
		t.Fatalf("seed partial %s: %v", key, err)
	}

	if etag == "" {
		return
	}

	if err := file.WriteFile(t.Context(), fsys, key+file.SourceETagSuffix,
		strings.NewReader(etag)); err != nil {
		t.Fatalf("seed etag for %s: %v", key, err)
	}
}

// present reports whether key is there, for the assertions about what a staged
// write leaves behind.
func present(t *testing.T, fsys fs.FS, key string) bool {
	t.Helper()

	_, err := fs.Stat(fsys, key)

	return err == nil
}

// TestPartialKeyIsDerivedFromTheCacheKey pins the naming, because two callers
// have to agree on it without sharing a variable: the one that writes a partial
// and the one that later looks for it.
func TestPartialKeyIsDerivedFromTheCacheKey(t *testing.T) {
	t.Parallel()

	if got, want := file.PartialKey("jpl/de440s.bsp"), "jpl/de440s.bsp.part"; got != want {
		t.Errorf("PartialKey = %q, want %q", got, want)
	}
}

// TestResumePointIsZeroWithoutAUsablePartial walks every way a partial can be
// unusable. All four answer 0, and the reason they must is the same: resuming
// from a partial that does not belong to the current source produces a file
// that is a prefix of one download and a suffix of another, which is corrupt
// in a way no size check would notice.
func TestResumePointIsZeroWithoutAUsablePartial(t *testing.T) {
	t.Parallel()

	const key = "jpl/de440s.bsp"

	t.Run("no partial at all", func(t *testing.T) {
		t.Parallel()

		if got := file.ResumePoint(t.Context(), newFS(t), key, `"etag"`); got != 0 {
			t.Errorf("ResumePoint with nothing staged = %d, want 0", got)
		}
	})

	t.Run("partial recorded no ETag", func(t *testing.T) {
		t.Parallel()

		fsys := newFS(t)
		seedPartial(t, fsys, file.PartialKey(key), "half", "")

		if got := file.ResumePoint(t.Context(), fsys, key, `"etag"`); got != 0 {
			t.Errorf("ResumePoint on a partial with no recorded ETag = %d, want 0 — "+
				"nothing says which source it came from", got)
		}
	})

	t.Run("source changed since the partial was written", func(t *testing.T) {
		t.Parallel()

		fsys := newFS(t)
		seedPartial(t, fsys, file.PartialKey(key), "half", `"old"`)

		if got := file.ResumePoint(t.Context(), fsys, key, `"new"`); got != 0 {
			t.Errorf("ResumePoint across an ETag change = %d, want 0 — resuming here "+
				"would splice two different downloads together", got)
		}

		// The unusable partial is discarded on the way, so the next attempt
		// does not re-examine it.
		if present(t, fsys, file.PartialKey(key)) {
			t.Error("a partial that no longer matches its source was left behind")
		}
	})
}

// TestResumePointReportsAMatchingPartialsSize is the case the feature exists
// for: the bytes already fetched are reused rather than fetched again.
func TestResumePointReportsAMatchingPartialsSize(t *testing.T) {
	t.Parallel()

	const (
		key  = "jpl/de440s.bsp"
		etag = `"unchanged"`
	)

	body := strings.Repeat("x", 4096)

	fsys := newFS(t)
	seedPartial(t, fsys, file.PartialKey(key), body, etag)

	if got, want := file.ResumePoint(t.Context(), fsys, key, etag), int64(len(body)); got != want {
		t.Errorf("ResumePoint = %d, want %d", got, want)
	}
}

// TestStageAndPromotePublishesOnlyCompleteContent is the invariant every reader
// of the cache depends on: nothing appears at the cache key until all of it is
// there and it has passed validation.
func TestStageAndPromotePublishesOnlyCompleteContent(t *testing.T) {
	t.Parallel()

	const (
		key  = "jpl/de440s.bsp"
		body = "a complete kernel"
	)

	fsys := newFS(t)

	if err := file.StageAndPromote(t.Context(), fsys, key, strings.NewReader(body), 0, `"etag"`, nil); err != nil {
		t.Fatalf("StageAndPromote: %v", err)
	}

	got, err := fs.ReadFile(fsys, key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != body {
		t.Errorf("promoted content = %q, want %q", got, body)
	}

	// The staging object is gone once the content is published; a leftover
	// would be resumed from on the next fetch of an already-complete file.
	if present(t, fsys, file.PartialKey(key)) {
		t.Error("the partial survived promotion")
	}
}

// TestStageAndPromoteResumesFromAnOffset covers the concatenation, and it is
// written so that a silent restart-from-zero fails rather than passes.
//
// The staged prefix is deliberately wrong content of the right length. If the
// offset is honoured, the result keeps that wrong prefix and appends only the
// remainder; a restart would produce the clean body instead, which is the
// answer that looks correct and means the feature did nothing.
func TestStageAndPromoteResumesFromAnOffset(t *testing.T) {
	t.Parallel()

	const (
		key    = "jpl/de440s.bsp"
		etag   = `"same"`
		staged = "XXXX"
		rest   = "-the-remainder"
	)

	fsys := newFS(t)
	seedPartial(t, fsys, file.PartialKey(key), staged, etag)

	offset := file.ResumePoint(t.Context(), fsys, key, etag)
	if offset != int64(len(staged)) {
		t.Fatalf("ResumePoint = %d, want %d", offset, len(staged))
	}

	if err := file.StageAndPromote(t.Context(), fsys, key, strings.NewReader(rest), offset, etag, nil); err != nil {
		t.Fatalf("StageAndPromote: %v", err)
	}

	got, err := fs.ReadFile(fsys, key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if want := staged + rest; string(got) != want {
		t.Errorf("promoted content = %q, want %q — the already-fetched bytes were not reused", got, want)
	}
}

// TestStageAndPromoteCachesNothingWhenValidationFails is why validation runs
// against the staged object rather than after promotion: a corrupt download
// must never become the cache entry, and a multi-gigabyte kernel must never
// have to fit in memory to be checked.
func TestStageAndPromoteCachesNothingWhenValidationFails(t *testing.T) {
	t.Parallel()

	const key = "jpl/de440s.bsp"

	fsys := newFS(t)

	err := file.StageAndPromote(t.Context(), fsys, key, strings.NewReader("truncated"), 0, `"etag"`,
		func(io.Reader) error { return errValidationFailed })
	if !errors.Is(err, errValidationFailed) {
		t.Fatalf("StageAndPromote = %v, want the validator's own error", err)
	}

	if present(t, fsys, key) {
		t.Error("content that failed validation was published to the cache key")
	}
}
