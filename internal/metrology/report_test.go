package metrology_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

func violatingSuite() *metrology.Suite {
	s := metrology.NewSuite("ephemeris.example", metrology.Reference{
		Kind:           metrology.KindSOFA,
		Name:           "gofa",
		Version:        "v1.19.1",
		Source:         "iauMoon98",
		SharedAncestor: "SOFA",
	}, metrology.MustContract(2.0, "arcsec",
		"the pointing budget for a 1 arcsec pixel scale",
		"docs/VALIDATION.md"))

	s.Add(metrology.Sample{Error: 0.5, Label: "Mars @ Paranal", Context: "2026-01-05"})
	s.Add(metrology.Sample{Error: 9.0, Label: "Moon @ Polar", Context: "2026-06-21 el=2.1"})

	return s
}

// A contract failure has to say enough that nobody has to open the source to
// act on it: which scenario, reproduced how, measured what, against what
// bound, justified by what, compared against what reference.
func TestReportFailureCarriesTheWholeStory(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	violatingSuite().Report(rec)

	if len(rec.errors) != 1 {
		t.Fatalf("got %d errors, want 1 (only one sample is outside the contract)", len(rec.errors))
	}

	for _, want := range []string{
		"Moon @ Polar",       // the scenario
		"2026-06-21 el=2.1",  // how to reproduce it
		"9",                  // what was measured
		"2",                  // the bound
		"arcsec",             // the units
		"pointing budget",    // why the bound has that value
		"docs/VALIDATION.md", // where the bound comes from
		"gofa",               // what it was compared against
	} {
		if !strings.Contains(rec.errors[0], want) {
			t.Errorf("failure message is missing %q:\n%s", want, rec.errors[0])
		}
	}

	// The passing sample must not be reported. A suite that lists every
	// comparison hides the ones that matter.
	if strings.Contains(rec.errors[0], "Mars @ Paranal") {
		t.Error("a sample inside the contract was reported as a failure")
	}
}

// The summary is logged whether or not anything failed. That is the premise
// of the package: a bound that never prints what it measured cannot say how
// much room is left under it.
func TestReportAlwaysLogsTheMeasurement(t *testing.T) {
	t.Parallel()

	s := metrology.NewSuite("ephemeris.passing", metrology.Reference{
		Kind: metrology.KindHorizons, Name: "JPL Horizons",
	}, testContract(100))
	s.Add(metrology.Sample{Error: 1, Label: "a", Context: "x"})
	s.Add(metrology.Sample{Error: 2, Label: "b", Context: "y"})

	rec := &recorder{}
	stats := s.Report(rec)

	if len(rec.errors) != 0 {
		t.Fatalf("a passing suite reported errors: %v", rec.errors)
	}

	out := rec.output()
	for _, want := range []string{"VERIFIED", "p50=", "p95=", "max=", "worst:", "headroom:"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary is missing %q:\n%s", want, out)
		}
	}

	if stats.N != 2 {
		t.Errorf("Report returned N = %d, want 2", stats.N)
	}
}

// Headroom is the number that would have caught both of this repository's
// unsound bounds: one pinned to its own measurement, one orders of magnitude
// looser than the reference's documented accuracy.
func TestReportShowsContractHeadroom(t *testing.T) {
	t.Parallel()

	s := metrology.NewSuite("x", metrology.Reference{}, testContract(10))
	s.Add(metrology.Sample{Error: 1, Label: "a", Context: "x"})

	rec := &recorder{}
	s.Report(rec)

	if !strings.Contains(rec.output(), "10x the measured max") {
		t.Errorf("headroom not reported as 10x:\n%s", rec.output())
	}
}

// Individual failures are capped so a broken contract cannot bury its own
// summary under thousands of lines.
func TestReportBoundsTheNumberOfListedViolations(t *testing.T) {
	t.Parallel()

	errs := make([]float64, 20)
	for i := range errs {
		errs[i] = 100
	}

	rec := &recorder{}
	testSuite(1, errs...).Report(rec)

	// Five listed individually, plus one line accounting for the rest.
	if len(rec.errors) != 6 {
		t.Fatalf("got %d errors, want 5 listed plus 1 summary", len(rec.errors))
	}

	if !strings.Contains(rec.errors[5], "15 further samples") {
		t.Errorf("the remainder is not accounted for: %q", rec.errors[5])
	}
}

// Nothing is written unless the operator asks for it, so an ordinary go test
// run leaves no files in anyone's working tree.
func TestReportWritesNothingWithoutTheEnvironmentVariable(t *testing.T) {
	dir := t.TempDir()

	t.Setenv(metrology.OutDirEnv, "")

	rec := &recorder{}
	violatingSuite().Report(rec)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("wrote %d files with %s unset", len(entries), metrology.OutDirEnv)
	}
}

func TestReportWritesAReadableDocument(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(metrology.OutDirEnv, dir)

	rec := &recorder{}
	violatingSuite().Report(rec)

	data, err := os.ReadFile(filepath.Join(dir, "ephemeris.example.json"))
	if err != nil {
		t.Fatalf("reading the written report: %v", err)
	}

	res, err := metrology.ReadResult(data)
	if err != nil {
		t.Fatalf("ReadResult: %v", err)
	}

	if res.Status != metrology.StatusViolated {
		t.Errorf("Status = %q, want %q", res.Status, metrology.StatusViolated)
	}

	if res.Stats.Max != 9 {
		t.Errorf("Stats.Max = %v, want 9", res.Stats.Max)
	}

	if res.Stats.Worst.Label != "Moon @ Polar" {
		t.Errorf("Stats.Worst.Label = %q, want the worst scenario", res.Stats.Worst.Label)
	}

	// Provenance has to survive the round trip, or the document ages into a
	// number with no claim attached.
	if res.Reference.SharedAncestor != "SOFA" {
		t.Errorf("SharedAncestor = %q, want SOFA", res.Reference.SharedAncestor)
	}

	if res.Contract.Rationale == "" || res.Contract.Source == "" {
		t.Error("the justification of the contract did not survive serialisation")
	}

	if res.Generated == "" || res.GoVersion == "" || res.Platform == "" {
		t.Errorf("provenance fields are empty: %+v", res)
	}
}

// A suite that could not run is recorded as not verified, never as absent.
//
// Absent reads identically to never having existed, so a service that dies
// permanently would leave the last good numbers on display with nothing to
// mark them stale.
func TestNotVerifiedIsRecordedRatherThanOmitted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(metrology.OutDirEnv, dir)

	rec := &recorder{}
	violatingSuite().NotVerified(rec, "JPL Horizons is unreachable")

	if !rec.skipped {
		t.Error("NotVerified did not skip the test")
	}

	if !strings.Contains(rec.skip, "NOT VERIFIED") || !strings.Contains(rec.skip, "unreachable") {
		t.Errorf("skip message = %q, want the reason in it", rec.skip)
	}

	data, err := os.ReadFile(filepath.Join(dir, "ephemeris.example.json"))
	if err != nil {
		t.Fatalf("a suite that did not run wrote no document: %v", err)
	}

	res, err := metrology.ReadResult(data)
	if err != nil {
		t.Fatalf("ReadResult: %v", err)
	}

	if res.Status != metrology.StatusNotVerified {
		t.Errorf("Status = %q, want %q", res.Status, metrology.StatusNotVerified)
	}

	if res.Status.OK() {
		t.Error("StatusNotVerified reports OK — an absence of evidence must not read as evidence")
	}

	if res.Reason != "JPL Horizons is unreachable" {
		t.Errorf("Reason = %q, want the reason it was skipped", res.Reason)
	}
}

// An old document is refused rather than reinterpreted: comparing two
// measurements taken under different definitions is worse than comparing
// nothing.
func TestReadResultRefusesAnotherSchemaVersion(t *testing.T) {
	t.Parallel()

	_, err := metrology.ReadResult([]byte("{\"schema_version\": 999, \"suite\": \"x\"}"))
	if !errors.Is(err, metrology.ErrSchemaVersion) {
		t.Errorf("error = %v, want %v", err, metrology.ErrSchemaVersion)
	}

	_, err = metrology.ReadResult([]byte("not json"))
	if !errors.Is(err, metrology.ErrDecodeReport) {
		t.Errorf("error = %v, want %v", err, metrology.ErrDecodeReport)
	}
}

// A suite name becomes a file name, so it must not be able to become a path.
func TestFileNameCannotEscapeItsDirectory(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ in, want string }{
		{"ephemeris.observer_precision", "ephemeris.observer_precision.json"},
		{"a/b", "a_b.json"},
		{"../../etc/passwd", ".._.._etc_passwd.json"},
		{"c:\\windows", "c__windows.json"},
		{"with spaces", "with_spaces.json"},
	} {
		if got := metrology.FileName(tc.in); got != tc.want {
			t.Errorf("FileName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
