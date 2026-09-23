package openngc

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/internal/testutil"
)

const (
	sampleNGCCSV = `Name;Type;RA;Dec;M;Common names;Identifiers;V-Mag;B-Mag
NGC1976;Nb;05:35:17.3;-05:23:28;42;Orion Nebula;;4.0;5.5
`
	sampleAddendumCSV = `Name;Type;RA;Dec;M;Common names;Identifiers;V-Mag;B-Mag
NGC0224;G;00:42:44.3;+41:16:09;31;Andromeda Galaxy;;3.4;4.4
`
)

// fakeSources opens a fresh temp directory as a remote.FS, points
// remote.OpenNGC's URL at it (SetURL), and writes both real OpenNGC
// source files (NGC.csv/addendum.csv) into it — a local stand-in for an
// HTTP source now that GetFile can't reach an http:// URL at all (no
// httpblob driver registered yet; see remote/file's package doc).
// fetchSource/New's own consent/caching policy is fully generic over any
// Bucket, so exercising it here tests the exact same code path an
// HTTP-backed endpoint will take once that driver exists.
func fakeSources(t *testing.T) remote.FS {
	t.Helper()

	dir := t.TempDir()

	url := testutil.FileURL(t, dir)

	if err := remote.SetURL(remote.OpenNGC, url); err != nil {
		t.Fatal(err)
	}

	fsys, err := remote.OpenFS(context.Background(), url)
	if err != nil {
		t.Fatalf("Open fake source: %v", err)
	}

	if err := remote.WriteFile(context.Background(), fsys, "NGC.csv", strings.NewReader(sampleNGCCSV)); err != nil {
		t.Fatalf("seed NGC.csv: %v", err)
	}

	if err := remote.WriteFile(context.Background(), fsys, "addendum.csv", strings.NewReader(sampleAddendumCSV)); err != nil {
		t.Fatalf("seed addendum.csv: %v", err)
	}

	return fsys
}

// loadNow builds a provider and drives its catalog load to completion.
//
// New performs no I/O — the fetch happens on the first query that needs the
// catalog — so a test about fetch/cache behavior has to ask a question to
// provoke one.
func loadNow(t *testing.T) {
	t.Helper()

	if _, err := New().Resolve(context.Background(), "M42"); err != nil {
		t.Fatalf("Resolve(M42): %v", err)
	}
}

func TestNewFetchesFromNetworkWhenDownloadsEnabled(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)

	remote.EnableDownloads(0, remote.OpenNGC)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	p := New()

	got, err := p.Resolve(context.Background(), "M42")
	if err != nil || got.ID != "NGC1976" {
		t.Errorf("Resolve(M42) = %+v, %v, want NGC1976, true", got, err)
	}

	// Regression: Epoch used to never be set despite OpenNGC's RA/Dec being
	// implicitly J2000 by the catalog's own convention.
	if !got.Epoch.Equal(time.J2000()) {
		t.Errorf("Epoch = %v, want time.J2000()", got.Epoch)
	}

	got, err = p.Resolve(context.Background(), "M31")
	if err != nil || got.ID != "NGC224" {
		t.Errorf("Resolve(M31) = %+v, %v, want NGC224, true", got, err)
	}
}

func TestLoadSkipsBodyWhenUnchanged(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)

	remote.EnableDownloads(0, remote.OpenNGC)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	loadNow(t)

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.OpenNGC)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	ngcAttrsBefore, err := fs.Stat(fsys, prefix+"NGC.csv")
	if err != nil {
		t.Fatalf("Attributes(NGC.csv): %v", err)
	}

	addendumAttrsBefore, err := fs.Stat(fsys, prefix+"addendum.csv")
	if err != nil {
		t.Fatalf("Attributes(addendum.csv): %v", err)
	}

	// Second load: the source is untouched (same mtime/size, so the same
	// fileblob-derived ETag), so both cache files must be reused untouched —
	// proved by their own ModTime staying identical, which only happens if
	// fetchInto's promote step never ran a second time.
	loadNow(t)

	ngcAttrsAfter, err := fs.Stat(fsys, prefix+"NGC.csv")
	if err != nil {
		t.Fatalf("Attributes(NGC.csv) after: %v", err)
	}

	addendumAttrsAfter, err := fs.Stat(fsys, prefix+"addendum.csv")
	if err != nil {
		t.Fatalf("Attributes(addendum.csv) after: %v", err)
	}

	if !ngcAttrsAfter.ModTime().Equal(ngcAttrsBefore.ModTime()) {
		t.Errorf("NGC.csv was rewritten on an unchanged-source New(): ModTime %v -> %v", ngcAttrsBefore.ModTime(), ngcAttrsAfter.ModTime())
	}

	if !addendumAttrsAfter.ModTime().Equal(addendumAttrsBefore.ModTime()) {
		t.Errorf("addendum.csv was rewritten on an unchanged-source New(): ModTime %v -> %v", addendumAttrsBefore.ModTime(), addendumAttrsAfter.ModTime())
	}
}

func TestNewDefaultDenyIssuesNoRequest(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	fakeSources(t)

	// Downloads intentionally left disabled (the default).
	p := New()

	if _, err := p.Resolve(context.Background(), "M42"); err == nil {
		t.Error("expected an empty provider when downloads are disabled")
	}

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.OpenNGC)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	// A failed stat is not "exists".
	if _, err := fs.Stat(fsys, prefix+"NGC.csv"); err == nil {
		t.Error("a denied query must not create a cache file")
	}
}

// TestLoadDoesNotAccumulateCacheFiles is a regression test: repeated loads
// must reuse a single cache file per source name, never leave stale versions
// behind (the concern that originally motivated fetchSource).
func TestLoadDoesNotAccumulateCacheFiles(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)

	remote.EnableDownloads(0, remote.OpenNGC)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	// Three separate providers, each loading for itself: the guard against
	// accumulation has to be the cache, not one provider's own memo.
	for range 3 {
		loadNow(t)
	}

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.OpenNGC)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	// OpenNGC is Mutable, so each cached CSV keeps an ETag sidecar recording
	// what the source said when it was fetched — that is what lets a reload
	// reuse the cache instead of re-downloading. A sidecar is bookkeeping, not
	// accumulation: the count stays fixed no matter how many times Load runs,
	// which is the property this test is actually about.
	wantNames := map[string]bool{
		"NGC.csv":           true,
		"NGC.csv.etag":      true,
		"addendum.csv":      true,
		"addendum.csv.etag": true,
	}
	got := testutil.BucketKeys(t, fsys, prefix)

	for _, key := range got {
		if !wantNames[key] {
			t.Errorf("unexpected cache object %q (possible version accumulation)", key)
		}
	}

	if len(got) != len(wantNames) {
		t.Errorf("expected exactly %d cache objects, got %d: %v", len(wantNames), len(got), got)
	}
}
