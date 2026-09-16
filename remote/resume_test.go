package remote

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote/file"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// resumeBody is the full payload the fake source serves; the tests below
// pre-seed a partial containing (deliberately altered) bytes standing in
// for its first N.
const resumeBody = "0123456789abcdefghijABCDEFGHIJ"

// fakeSource opens a fresh temp directory as a FS, points id's
// endpoint URL at it (SetURL), and writes content at name — a local
// stand-in for an HTTP source now that GetFile can't reach an http://
// URL at all (no httpblob driver registered yet; see remote/file's
// package doc). fetchInto/freshInCache/unchanged/ResumePoint/
// writeResumable are all fully generic over any Bucket, so exercising
// them against a real local Bucket here tests the exact same policy
// code an HTTP-backed endpoint will take once that driver exists — only
// the literal HTTP Range/If-Range wire format is untestable until then.
// fileblob's own Attributes derives a real, content-sensitive ETag from
// (ModTime, Size), so the unchanged()/resume validator-comparison logic
// gets genuine, non-trivial coverage, not a stub.
func fakeSource(t *testing.T, id EndpointID, name, content string) FS {
	t.Helper()

	dir := t.TempDir()

	url := testutil.FileURL(t, dir)

	if err := SetURL(id, url); err != nil {
		t.Fatal(err)
	}

	fsys, err := file.OpenFS(url)
	if err != nil {
		t.Fatalf("Open fake source: %v", err)
	}

	if err := WriteFile(context.Background(), fsys, name, strings.NewReader(content)); err != nil {
		t.Fatalf("seed fake source: %v", err)
	}

	return fsys
}

// seedPartial writes a partial body (and, when validator != "", the
// "source-etag" Metadata ResumePoint reads back) into cacheKey's ".part"
// key under NAIFSPK's cache directory, simulating a download that was
// interrupted partway.
func seedPartial(t *testing.T, content, validator string) {
	t.Helper()

	const name = "kernel.bsp"

	fsys, prefix, err := CacheDir(context.Background(), NAIFSPK)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	pKey := file.PartialKey(prefix + name)

	// The partial body and the ETag sidecar that records which source it came
	// from — two ordinary writes, where file.SavePartial used to carry the ETag
	// as gocloud object metadata. io/fs has no metadata; see
	// file.SourceETagSuffix.
	if err := WriteFile(context.Background(), fsys, pKey, strings.NewReader(content)); err != nil {
		t.Fatalf("seed partial: %v", err)
	}

	if validator != "" {
		if err := WriteFile(context.Background(), fsys, pKey+file.SourceETagSuffix,
			strings.NewReader(validator)); err != nil {
			t.Fatalf("seed partial etag: %v", err)
		}
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

	srcFS := fakeSource(t, NAIFSPK, "kernel.bsp", resumeBody)

	info, err := fs.Stat(srcFS, "kernel.bsp")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	EnableDownloads(0, NAIFSPK)

	seededPrefix := strings.Repeat("X", done)
	seedPartial(t, seededPrefix, file.ETag(info))

	fsys, key, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, err := fs.ReadFile(fsys, key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	// The whole point: seeded (wrong) prefix + resumed real remainder,
	// with no duplicated or dropped bytes at the seam.
	want := seededPrefix + resumeBody[done:]
	if string(got) != want {
		t.Errorf("resumed file = %q, want %q (seeded prefix preserved + genuine remainder)", got, want)
	}

	assertPartialCleared(t, fsys, key)
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

	fsys, key, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, err := fs.ReadFile(fsys, key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != resumeBody {
		t.Errorf("restarted file = %q, want exactly %q with no stale prefix", got, resumeBody)
	}

	if strings.Contains(string(got), "STALE") {
		t.Error("stale partial bytes survived into the final file")
	}

	assertPartialCleared(t, fsys, key)
}

// TestGetFileIgnoresPartialWithoutValidator verifies a partial with no
// recorded validator is discarded rather than resumed — without one
// there is no way to prove it still matches the source.
func TestGetFileIgnoresPartialWithoutValidator(t *testing.T) {
	cleanRemoteState(t)

	fakeSource(t, NAIFSPK, "kernel.bsp", resumeBody)

	EnableDownloads(0, NAIFSPK)
	seedPartial(t, strings.Repeat("X", 10), "") // wrong-prefix body, no validator

	fsys, key, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, err := fs.ReadFile(fsys, key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != resumeBody {
		t.Errorf("file = %q, want %q — a validator-less partial must not be resumed", got, resumeBody)
	}

	assertPartialCleared(t, fsys, key)
}

// TestGetFileResumeRespectsConsentSize verifies the consent gate is applied
// to the file's FULL size, not just the remaining leg — otherwise resuming
// would be a way to slip past a MaxDownloadSize cap a few bytes at a time.
func TestGetFileResumeRespectsConsentSize(t *testing.T) {
	cleanRemoteState(t)

	const done = 25 // only 5 bytes remain, but the file is 30

	srcFS := fakeSource(t, NAIFSPK, "kernel.bsp", resumeBody)

	info, err := fs.Stat(srcFS, "kernel.bsp")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	// Cap below the full size but above the remaining bytes.
	EnableDownloads(10, NAIFSPK)
	seedPartial(t, resumeBody[:done], file.ETag(info))

	_, _, err = GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied (full size %d exceeds the 10-byte cap), got %v", len(resumeBody), err)
	}
}

// assertPartialCleared verifies a completed download leaves nothing behind.
//
// Two objects now rather than one: the ETag used to ride as gocloud metadata on
// the partial key, so a single check covered both. It is a sidecar now, and a
// sidecar that outlives its partial is exactly the leftover this asserts
// against — ResumePoint would read it, find no body, and start over, but the
// file would sit in the cache forever.
func assertPartialCleared(t *testing.T, fsys FS, cacheKey string) {
	t.Helper()

	pKey := file.PartialKey(cacheKey)

	for _, key := range []string{pKey, pKey + file.SourceETagSuffix} {
		if _, err := fs.Stat(fsys, key); err == nil {
			t.Errorf("%s survived a completed download", key)
		}
	}
}
