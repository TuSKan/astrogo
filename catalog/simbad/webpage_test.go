package simbad

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// maintenancePage is the shape an archive actually serves when it is down: a
// human-readable notice, with a 200, in place of the result set. The commas are
// deliberate — they are what let such a page survive a CSV reader far enough to
// be mistaken for data.
const maintenancePage = `<!DOCTYPE html>
<html lang="en">
<head><title>Service unavailable</title></head>
<body>
<h1>The archive is temporarily unavailable</h1>
<p>Scheduled maintenance. Status, updates, and contact details below.</p>
<ul>
<li>Mirrors: CDS, Harvard, Beijing</li>
</ul>
</body>
</html>
`

// TestAWebPageIsReportedAsDowntime is #300: a caller must be able to tell "the
// archive is down" from "the archive sent nonsense", because the two want
// opposite handling — back off and retry, versus stop, because retrying will
// not help. Both used to arrive as an opaque error string.
//
// # What was measured, and what it changed
//
// The issue suspected the CSV providers were the worse case, on the reasoning
// that a web page handed to encoding/csv is lines of text with commas in them
// and might yield rows rather than an error. Measured, that is not what
// happens: these parsers look their columns up by name and a web page has none,
// so both stopped. No fabricated target ever reached a caller.
//
// What they reported was `missing expected column: "main_id"`, which is the
// service-changed-its-schema answer to a service-is-down question — the same
// misdiagnosis #301 fixed on the VOTable side, where an HTML page surfaced as
// an XML syntax error and pointed at a parser bug that does not exist.
//
// So on this path the fix is legibility rather than safety, and it is worth
// saying plainly: nothing was returning wrong data, and something was sending
// every reader after the wrong problem.
func TestAWebPageIsReportedAsDowntime(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		parse func(r io.Reader) ([]resolve.Target, error)
	}{
		{"ParseCSV", ParseCSV},
		{"ParseBrightCSV", ParseBrightCSV},
	} {
		_, err := tc.parse(strings.NewReader(maintenancePage))
		if err == nil {
			t.Errorf("%s: a web page parsed without error", tc.name)
			continue
		}

		if !errors.Is(err, remote.ErrNotServingData) {
			t.Errorf("%s: err = %v, which does not match remote.ErrNotServingData — "+
				"a caller cannot tell downtime from corruption", tc.name, err)
		}
	}
}

// TestRealDataStillParses is the other half, and the one that would hurt: a
// guard that rejected good responses would turn every successful query into a
// reported outage.
func TestRealDataStillParses(t *testing.T) {
	t.Parallel()

	const rows = "oid,main_id,ra,dec,otype,id\n" +
		"1,M  31,10.6847083,41.2687500,G,M 31\n" +
		"2,M  33,23.4620417,30.6602222,G,M 33\n"

	out, err := ParseCSV(strings.NewReader(rows))
	if err != nil {
		t.Fatalf("a valid CSV response failed to parse: %v", err)
	}

	if len(out) != 2 {
		t.Errorf("got %d targets, want 2", len(out))
	}
}

// TestAMalformedResponseIsNotReportedAsDowntime keeps the distinction pointing
// both ways. A genuinely broken payload must not claim the archive is down,
// since a caller reading that would retry forever against a service that is
// working fine.
func TestAMalformedResponseIsNotReportedAsDowntime(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		body string
	}{
		{"headers the provider does not recognize", "alpha,beta,gamma\n1,2,3\n"},
		{"a truncated row", "oid,main_id,ra,dec,otype,id\n1,M  31\n"},
		{"not tabular at all", "{\"error\": \"nope\"}\n"},
		{"empty", ""},
		{"XML, which is somebody else's format but not a web page", "<?xml version=\"1.0\"?><VOTABLE/>"},
	} {
		_, err := ParseCSV(strings.NewReader(tc.body))
		if err == nil {
			continue // Tolerated by the parser; not this test's subject.
		}

		if errors.Is(err, remote.ErrNotServingData) {
			t.Errorf("%s: reported as the service not serving data, but the service did "+
				"serve something — retrying will not fix this: %v", tc.name, err)
		}
	}
}
