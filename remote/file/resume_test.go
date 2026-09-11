package file_test

import (
	"errors"
	"io"
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
// and the defect. These functions take a bucket and a key and nothing else, so
// they can be asked directly what they do — and the questions worth asking are
// about a partial nobody wrote, a partial that no longer matches its source,
// and content that fails validation after it was already staged.

// newBucket opens a fresh temp directory as a bucket.
func newBucket(t *testing.T) *file.Bucket {
	t.Helper()

	b, err := file.Open(t.Context(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return b
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

		if got := file.ResumePoint(t.Context(), newBucket(t), key, `"etag"`); got != 0 {
			t.Errorf("ResumePoint with nothing staged = %d, want 0", got)
		}
	})

	t.Run("partial recorded no ETag", func(t *testing.T) {
		t.Parallel()

		bucket := newBucket(t)
		if err := file.SavePartial(t.Context(), bucket, file.PartialKey(key), strings.NewReader("half"), ""); err != nil {
			t.Fatalf("SavePartial: %v", err)
		}

		if got := file.ResumePoint(t.Context(), bucket, key, `"etag"`); got != 0 {
			t.Errorf("ResumePoint on a partial with no recorded ETag = %d, want 0 — "+
				"nothing says which source it came from", got)
		}
	})

	t.Run("source changed since the partial was written", func(t *testing.T) {
		t.Parallel()

		bucket := newBucket(t)
		if err := file.SavePartial(t.Context(), bucket, file.PartialKey(key), strings.NewReader("half"), `"old"`); err != nil {
			t.Fatalf("SavePartial: %v", err)
		}

		if got := file.ResumePoint(t.Context(), bucket, key, `"new"`); got != 0 {
			t.Errorf("ResumePoint across an ETag change = %d, want 0 — resuming here "+
				"would splice two different downloads together", got)
		}

		// The unusable partial is discarded on the way, so the next attempt
		// does not re-examine it.
		if exists, _ := bucket.Exists(t.Context(), file.PartialKey(key)); exists {
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

	bucket := newBucket(t)
	if err := file.SavePartial(t.Context(), bucket, file.PartialKey(key), strings.NewReader(body), etag); err != nil {
		t.Fatalf("SavePartial: %v", err)
	}

	if got, want := file.ResumePoint(t.Context(), bucket, key, etag), int64(len(body)); got != want {
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

	bucket := newBucket(t)

	if err := file.StageAndPromote(t.Context(), bucket, key, strings.NewReader(body), 0, `"etag"`, nil); err != nil {
		t.Fatalf("StageAndPromote: %v", err)
	}

	got, err := bucket.ReadAll(t.Context(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != body {
		t.Errorf("promoted content = %q, want %q", got, body)
	}

	// The staging object is gone once the content is published; a leftover
	// would be resumed from on the next fetch of an already-complete file.
	if exists, _ := bucket.Exists(t.Context(), file.PartialKey(key)); exists {
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

	bucket := newBucket(t)
	if err := file.SavePartial(t.Context(), bucket, file.PartialKey(key), strings.NewReader(staged), etag); err != nil {
		t.Fatalf("SavePartial: %v", err)
	}

	offset := file.ResumePoint(t.Context(), bucket, key, etag)
	if offset != int64(len(staged)) {
		t.Fatalf("ResumePoint = %d, want %d", offset, len(staged))
	}

	if err := file.StageAndPromote(t.Context(), bucket, key, strings.NewReader(rest), offset, etag, nil); err != nil {
		t.Fatalf("StageAndPromote: %v", err)
	}

	got, err := bucket.ReadAll(t.Context(), key)
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

	bucket := newBucket(t)

	err := file.StageAndPromote(t.Context(), bucket, key, strings.NewReader("truncated"), 0, `"etag"`,
		func(io.Reader) error { return errValidationFailed })
	if !errors.Is(err, errValidationFailed) {
		t.Fatalf("StageAndPromote = %v, want the validator's own error", err)
	}

	if exists, _ := bucket.Exists(t.Context(), key); exists {
		t.Error("content that failed validation was published to the cache key")
	}
}
