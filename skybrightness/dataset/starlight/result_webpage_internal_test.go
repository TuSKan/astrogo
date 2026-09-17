package starlight

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestAWebPageIsReportedAsDowntime is #300 from the dataset tier.
//
// The Gaia archive serves its maintenance page with a 200, and before this the
// result was ErrGaiaResponse wrapping an XML syntax error — indistinguishable
// from a genuinely corrupt VOTable, which is the one case where retrying is
// pointless.
//
// Both sentinels are asserted. ErrGaiaResponse is what this package has always
// reported and what callers already match; remote.ErrNotServingData says which
// kind of unreadable it is. Dropping either would break somebody.
//
// This is also why the sentinel lives in remote rather than on catalog/resolve:
// skybrightness cannot import catalog without inverting the layering, so a
// resolve sentinel would have forced this package to invent a second name for
// the same condition.
func TestAWebPageIsReportedAsDowntime(t *testing.T) {
	t.Parallel()

	const page = `<!DOCTYPE html>
<html lang="en"><head><title>Gaia archive</title></head>
<body><h1>Undergoing maintenance</h1></body></html>
`

	_, err := newVOTableRows(strings.NewReader(page))
	if err == nil {
		t.Fatal("a web page parsed as a VOTable without error")
	}

	if !errors.Is(err, ErrGaiaResponse) {
		t.Errorf("err = %v, which no longer matches ErrGaiaResponse", err)
	}

	if !errors.Is(err, remote.ErrNotServingData) {
		t.Errorf("err = %v, which does not match remote.ErrNotServingData — a caller "+
			"cannot tell the archive being down from the archive sending nonsense", err)
	}
}

// TestAMalformedVOTableIsNotReportedAsDowntime is the paired negative: a broken
// document must not claim the service is down, or a caller retries forever
// against an archive that is working.
func TestAMalformedVOTableIsNotReportedAsDowntime(t *testing.T) {
	t.Parallel()

	const broken = `<?xml version="1.0"?><VOTABLE><RESOURCE><TABLE><DATA><TABLEDATA><TR><TD>1`

	_, err := newVOTableRows(strings.NewReader(broken))
	if err == nil {
		t.Skip("this document parsed cleanly; not a useful negative case")
	}

	if errors.Is(err, remote.ErrNotServingData) {
		t.Errorf("a malformed VOTable was reported as the service not serving data: %v", err)
	}
}
