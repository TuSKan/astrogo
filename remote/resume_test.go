package remote

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gocloud.dev/blob"

	"github.com/TuSKan/astrogo/remote/file"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// resumeBody is the full payload the fake source serves; the tests below
// pre-seed a partial containing (deliberately altered) bytes standing in
// for its first N.
const resumeBody = "0123456789abcdefghijABCDEFGHIJ"

// fakeSource opens a fresh temp directory as a *file.Bucket, points id's
// endpoint URL at it (SetURL), and writes content at name — a local
// stand-in for an HTTP source now that GetFile can't reach an http://
// URL at all (no httpblob driver registered yet; see remote/file's
// package doc). fetchInto/freshInCache/unchanged/resumePoint/
// writeResumable are all fully generic over any Bucket, so exercising
// them against a real local Bucket here tests the exact same policy
// code an HTTP-backed endpoint will take once that driver exists — only
// the literal HTTP Range/If-Range wire format is untestable until then.
// fileblob's own Attributes derives a real, content-sensitive ETag from
// (ModTime, Size), so the unchanged()/resume validator-comparison logic
// gets genuine, non-trivial coverage, not a stub.
func fakeSource(t *testing.T, id EndpointID, name, content string) *file.Bucket {
	t.Helper()

	dir := t.TempDir()

	url := testutil.FileURL(t, dir)

	if err := SetURL(id, url); err != nil {
		t.Fatal(err)
	}

	bucket, err := file.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("Open fake source: %v", err)
	}

	if err := bucket.WriteAll(context.Background(), name, []byte(content), nil); err != nil {
		t.Fatalf("seed fake source: %v", err)
	}

	return bucket
}

// seedPartial writes a partial body (and, when validator != "", the
// "source-etag" Metadata resumePoint reads back) into cacheKey's ".part"
// key under NAIFSPK's cache directory, simulating a download that was
// interrupted partway.
func seedPartial(t *testing.T, content, validator string) {
	t.Helper()

	const name = "kernel.bsp"

	bucket, prefix, err := CacheDir(context.Background(), NAIFSPK)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	pKey := partialKey(prefix + name)

	var opts *blob.WriterOptions
	if validator != "" {
		opts = &blob.WriterOptions{Metadata: map[string]string{sourceETagKey: validator}}
	}

	if err := bucket.WriteAll(context.Background(), pKey, []byte(content), opts); err != nil {
		t.Fatalf("seed partial: %v", err)
	}
}

// TestGetFileResumesFromPartial is the core of the feature: an interrupted
// download must continue from where it stopped, not re-fetch from zero.
//
// The seeded partial's first `done` bytes are deliberately WRONG (an "X"
// run, not resumeBody's real prefix) but the right length, with a
// validator matching the fake source's real current ETag. If GetFile
// truly resumes (rather than silently restarting from zero), the final
// cached content keeps this wrong prefix and appends only the genuinely-
// fetched remainder; a fresh restart would produce the real resumeBody
// instead, with no trace of the seeded prefix — a more direct proof of
// byte-level resume behavior than inspecting request headers ever was.
func TestGetFileResumesFromPartial(t *testing.T) {
	cleanRemoteState(t)

	const done = 10

	srcBucket := fakeSource(t, NAIFSPK, "kernel.bsp", resumeBody)

	attrs, err := srcBucket.Attributes(context.Background(), "kernel.bsp")
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	EnableDownloads(0, NAIFSPK)

	seededPrefix := strings.Repeat("X", done)
	seedPartial(t, seededPrefix, attrs.ETag)

	bucket, key, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, err := bucket.ReadAll(context.Background(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	// The whole point: seeded (wrong) prefix + resumed real remainder,
	// with no duplicated or dropped bytes at the seam.
	want := seededPrefix + resumeBody[done:]
	if string(got) != want {
		t.Errorf("resumed file = %q, want %q (seeded prefix preserved + genuine remainder)", got, want)
	}

	assertPartialCleared(t, bucket, key)
}

// TestGetFileRestartsWhenValidatorStale verifies the validator safety net:
// if the recorded validator no longer matches the source's current ETag,
// the stale partial must be discarded and the full file re-fetched, not
// appended to (which would splice old and new bytes into a corrupt file).
func TestGetFileRestartsWhenValidatorStale(t *testing.T) {
	cleanRemoteState(t)

	fakeSource(t, NAIFSPK, "kernel.bsp", resumeBody)

	EnableDownloads(0, NAIFSPK)
	seedPartial(t, "STALE-BYTES", `"stale-etag-does-not-match-anything"`)

	bucket, key, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, err := bucket.ReadAll(context.Background(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != resumeBody {
		t.Errorf("restarted file = %q, want exactly %q with no stale prefix", got, resumeBody)
	}

	if strings.Contains(string(got), "STALE") {
		t.Error("stale partial bytes survived into the final file")
	}

	assertPartialCleared(t, bucket, key)
}

// TestGetFileIgnoresPartialWithoutValidator verifies a partial with no
// recorded validator is discarded rather than resumed — without one
// there is no way to prove it still matches the source.
func TestGetFileIgnoresPartialWithoutValidator(t *testing.T) {
	cleanRemoteState(t)

	fakeSource(t, NAIFSPK, "kernel.bsp", resumeBody)

	EnableDownloads(0, NAIFSPK)
	seedPartial(t, strings.Repeat("X", 10), "") // wrong-prefix body, no validator

	bucket, key, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, err := bucket.ReadAll(context.Background(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != resumeBody {
		t.Errorf("file = %q, want %q — a validator-less partial must not be resumed", got, resumeBody)
	}

	assertPartialCleared(t, bucket, key)
}

// TestGetFileResumeRespectsConsentSize verifies the consent gate is applied
// to the file's FULL size, not just the remaining leg — otherwise resuming
// would be a way to slip past a MaxDownloadSize cap a few bytes at a time.
func TestGetFileResumeRespectsConsentSize(t *testing.T) {
	cleanRemoteState(t)

	const done = 25 // only 5 bytes remain, but the file is 30

	srcBucket := fakeSource(t, NAIFSPK, "kernel.bsp", resumeBody)

	attrs, err := srcBucket.Attributes(context.Background(), "kernel.bsp")
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	// Cap below the full size but above the remaining bytes.
	EnableDownloads(10, NAIFSPK)
	seedPartial(t, resumeBody[:done], attrs.ETag)

	_, _, err = GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied (full size %d exceeds the 10-byte cap), got %v", len(resumeBody), err)
	}
}

// assertPartialCleared verifies a completed download leaves no sidecar —
// the resume validator rides as Metadata on the partial key itself, so a
// single Exists check on that key covers both.
func assertPartialCleared(t *testing.T, bucket *file.Bucket, cacheKey string) {
	t.Helper()

	pKey := partialKey(cacheKey)
	// A failed existence check is not "exists".
	if exists, _ := bucket.Exists(context.Background(), pKey); exists {
		t.Errorf("partial %s survived a completed download", pKey)
	}
}
