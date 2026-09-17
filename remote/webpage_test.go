package remote_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// utf8BOM is the byte order mark some services emit ahead of a document,
// written as bytes because a Go source file may not contain one.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// TestLooksLikeHTMLSeparatesAnErrorPageFromAPayload covers the heuristic in
// both directions, and the second direction is the one that matters: a false
// positive would turn a good result into a reported outage, which is worse
// than the misdiagnosis this exists to prevent.
func TestLooksLikeHTMLSeparatesAnErrorPageFromAPayload(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"a doctype", "<!DOCTYPE html>\n<html><body>Down for maintenance</body></html>", true},
		{"lowercase doctype", "<!doctype html><html></html>", true},
		{"a bare html tag", "<html lang=\"en\"><head><title>503</title></head></html>", true},
		{"uppercase, as older pages still are", "<HTML><BODY>Service unavailable</BODY></HTML>", true},
		{"leading blank lines, which some services emit", "\n\n  <!DOCTYPE html>\n<html>", true},
		{"a byte order mark ahead of the doctype", string(utf8BOM) + "<!DOCTYPE html><html>", true},

		// The negatives. A VOTable is XML, so anything keying on "<" would
		// classify every good Gaia response as an outage.
		{
			name: "a VOTable, which is XML and must not match",
			body: `<?xml version="1.0"?><VOTABLE version="1.3"><RESOURCE><TABLE></TABLE></RESOURCE></VOTABLE>`,
			want: false,
		},
		{
			name: "a VOTable with no XML declaration",
			body: `<VOTABLE version="1.4"><RESOURCE/></VOTABLE>`,
			want: false,
		},
		{"a CSV header", "main_id,ra,dec,otype\nM31,10.68,41.27,Galaxy", false},
		{"a CSV row that happens to contain a tag", "name,note\nNGC224,\"<html> in a comment\"", false},
		{"empty", "", false},
		{"whitespace only", "   \n\t\n", false},
		{"JSON", `{"error":"service unavailable"}`, false},
		{"a plain-text notice", "Service temporarily unavailable. Try again later.", false},
	} {
		if got := remote.LooksLikeHTML([]byte(tc.body)); got != tc.want {
			t.Errorf("%s: LooksLikeHTML = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestLooksLikeHTMLReadsOnlyThePrefix checks that a large body costs nothing
// beyond its opening bytes, since callers pass whatever they peeked and one of
// them could reasonably hand over a whole document.
func TestLooksLikeHTMLReadsOnlyThePrefix(t *testing.T) {
	t.Parallel()

	// A VOTable whose payload is megabytes of rows: still not HTML, and the
	// answer must not depend on how much of it was passed.
	big := `<?xml version="1.0"?><VOTABLE>` + strings.Repeat("<TR><TD>1</TD></TR>", 200000)

	if remote.LooksLikeHTML([]byte(big)) {
		t.Error("a large VOTable was classified as a web page")
	}

	// And an HTML page is recognised from its opening bytes alone, so a caller
	// peeking a few hundred bytes gets the same answer as one passing it all.
	page := "<!DOCTYPE html>\n" + strings.Repeat("<p>maintenance</p>", 100000)

	if !remote.LooksLikeHTML([]byte(page)[:64]) {
		t.Error("an HTML page was not recognised from its first 64 bytes")
	}

	if !remote.LooksLikeHTML([]byte(page)) {
		t.Error("an HTML page was not recognised when passed whole")
	}
}

// TestErrNotServingDataIsDistinctFromTheOtherRemoteSentinels guards the thing a
// caller branches on: this must not be confused with offline mode or a denied
// download, all three of which mean "no data" for entirely different reasons
// and want entirely different handling.
func TestErrNotServingDataIsDistinctFromTheOtherRemoteSentinels(t *testing.T) {
	t.Parallel()

	for _, other := range []error{
		remote.ErrOffline,
		remote.ErrDownloadDenied,
		remote.ErrEndpointDisabled,
		remote.ErrUnknownEndpoint,
	} {
		// Wrapped the way a provider wraps it, so this asks the question a
		// caller actually asks — "is this error that condition" — rather than
		// comparing two bare sentinels, which reads as reversed arguments and
		// tests a weaker thing.
		wrapped := fmt.Errorf("provider: %w", other)

		if errors.Is(wrapped, remote.ErrNotServingData) {
			t.Errorf("an error wrapping %v matches ErrNotServingData", other)
		}
	}
}
