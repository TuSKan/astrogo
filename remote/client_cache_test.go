package remote

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestEachClientOpensItsOwnCache pins the half of #114 that survives #272.
//
// This property had a home in storage_staging_test.go, which tested it
// alongside fileblob's staging parameter. #272 removed that parameter — it
// fixed one collision and caused a worse one across processes — and deleted
// the file with it. The per-client half is not the parameter's, it is this
// change's, and it is the one a merge of the two is most likely to get wrong.
//
// The wrong resolution compiles and passes everything else: have DataDir open
// the package-level DataDirURL, which reads Default. Every client would then
// read and write the process cache, so a component handed its own cache
// directory would silently share the default one — which is the exact failure
// remote.Client exists to remove, reintroduced by a conflict resolution rather
// than by anybody deciding it.
func TestEachClientOpensItsOwnCache(t *testing.T) {
	// Not parallel: it reads and writes Default.
	t.Cleanup(func() { SetDataDir("") })

	// Real directories, because the second half of this test opens them.
	defaultURL := testutil.FileURL(t, t.TempDir())
	ownURL := testutil.FileURL(t, t.TempDir())

	SetDataDir(defaultURL)

	own := NewClient(WithCacheURL(ownURL))

	if got := own.DataDirURL(); got != ownURL {
		t.Errorf("the client resolves %q, which is not the cache it was given.\n"+
			"  A client's cache location has to reach the URL it actually opens.", got)
	}

	if got := Default().DataDirURL(); got != defaultURL {
		t.Errorf("Default now resolves %q.\n"+
			"  Constructing a client with its own cache must not move the process one.", got)
	}

	// And the opener, not just the accessor. The two assertions above pin
	// DataDirURL, which cannot catch a DataDir that resolves the right URL and
	// then opens the wrong one — measured: mutating DataDir to call the
	// package-level DataDirURL leaves them both passing.
	//
	// file.Open memoises one Bucket per URL, so two clients pointed at
	// different caches must not come back with the same pointer.
	ownBucket, err := own.DataDir(t.Context())
	if err != nil {
		t.Fatalf("own.DataDir: %v", err)
	}

	defaultBucket, err := Default().DataDir(t.Context())
	if err != nil {
		t.Fatalf("Default().DataDir: %v", err)
	}

	if ownBucket == defaultBucket {
		t.Error("the client and the default opened the same bucket.\n" +
			"  DataDir has to open the client's own cache; resolving through the " +
			"package-level DataDirURL sends every client to the process one.")
	}
}
