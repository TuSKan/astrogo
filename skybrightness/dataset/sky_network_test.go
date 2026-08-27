//go:build network

package dataset_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
)

// A refused download says which call grants it.
//
// # Why this needs a test at all
//
// Because it is a designed behaviour rather than an incidental error string.
// Consent deliberately stays outside this package — a convenience that
// granted its own would fetch 145 MB because somebody typed a preset name —
// and the price of that decision is that a first-time caller hits a refusal.
// The message paying that price is the whole mitigation, and until this test
// existed nothing had ever executed it.
func TestOpenNamesTheConsentItNeeds(t *testing.T) {
	testutil.RequireReachable(t, "svo2.cab.inta-csic.es:443")

	// A cache of this test's own. With a warm one Open succeeds with no
	// consent at all, because an immutable object is reused on existence
	// alone — correct, and enough to make the first version of this test pass
	// on a cold machine and fail on the one that had just run the example.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	_, err := dataset.Open(context.Background(), dataset.Spec{
		Preset: skybrightness.GAMBONSWeb,
	})
	if err == nil {
		t.Fatal("Open succeeded with no consent granted")
	}

	// Consent gates downloads, not every request, so this test needs a
	// network: Inputs resolves the passband from an API first, and only then
	// reaches the star map, which is the first thing consent applies to.
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("got %v, want ErrDownloadDenied", err)
	}

	for _, want := range []string{"EnableDownloads", "dataset.Endpoints", "gambons-web"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q, so a caller is told they cannot "+
				"proceed without being told how:\n%v", want, err)
		}
	}
}
