package vizier

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestAWebPageIsReportedAsDowntime is #300 on the VizieR side.
//
// VizieR answers a TAP query with CSV, and serves its own failures as an HTML
// page with a 200. The parser did stop on such a page — it looks its columns up
// by name and a web page has none — but it stopped on the column check,
// reporting a missing "designation" column. That is the
// service-changed-its-schema answer to a service-is-down question, and a caller
// reading it would go looking for a VizieR schema change that never happened.
func TestAWebPageIsReportedAsDowntime(t *testing.T) {
	t.Parallel()

	const page = `<!DOCTYPE html>
<html lang="en">
<head><title>VizieR</title></head>
<body><h1>The TAP service is temporarily unavailable</h1>
<p>Mirrors: CDS, Cambridge, Beijing</p></body>
</html>
`

	_, err := parseCSV(strings.NewReader(page), tableSchemas[defaultTable])
	if err == nil {
		t.Fatal("a web page parsed as CSV without error")
	}

	if !errors.Is(err, remote.ErrNotServingData) {
		t.Errorf("err = %v, which does not match remote.ErrNotServingData", err)
	}

	// And it must not also claim the schema changed, which is the wrong
	// diagnosis this replaces.
	if errors.Is(err, ErrUnexpectedSchema) {
		t.Errorf("err = %v, which still blames the schema", err)
	}
}

// TestRealCSVStillParses is the paired negative: a guard that rejected good
// responses would turn every successful cone search into a reported outage.
func TestRealCSVStillParses(t *testing.T) {
	t.Parallel()

	const rows = "designation,ra,dec\n" +
		"2MASS J00424433+4116074,10.684708,41.268750\n" +
		"2MASS J01335090+3039357,23.462042,30.660222\n"

	out, err := parseCSV(strings.NewReader(rows), tableSchemas[defaultTable])
	if err != nil {
		t.Fatalf("a valid CSV response failed to parse: %v", err)
	}

	if len(out) != 2 {
		t.Errorf("got %d targets, want 2", len(out))
	}
}

// TestAChangedSchemaIsStillAChangedSchema keeps the two diagnoses apart in the
// other direction: a response that really is CSV with the wrong columns must
// still report the schema, not downtime, or a caller retries forever against a
// service that is answering perfectly well.
func TestAChangedSchemaIsStillAChangedSchema(t *testing.T) {
	t.Parallel()

	const renamed = "source_id,ra_deg,dec_deg\n1,10.68,41.27\n"

	_, err := parseCSV(strings.NewReader(renamed), tableSchemas[defaultTable])
	if err == nil {
		t.Fatal("a response with unrecognised columns parsed without error")
	}

	if !errors.Is(err, ErrUnexpectedSchema) {
		t.Errorf("err = %v, want ErrUnexpectedSchema", err)
	}

	if errors.Is(err, remote.ErrNotServingData) {
		t.Errorf("err = %v, which blames the service for a schema problem", err)
	}
}
