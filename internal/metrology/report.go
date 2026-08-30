package metrology

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TuSKan/astrogo/time"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// SchemaVersion is the version of the result document written by [Suite].
//
// It is checked on read and never guessed at: a reader that silently
// reinterprets an older document produces a comparison between two things
// that were not measured the same way, which is worse than refusing.
const SchemaVersion = 1

// OutDirEnv names the environment variable that turns report writing on.
//
// Reporting is opt-in so an ordinary "go test" run behaves exactly as it did
// before a suite was retrofitted — no files appear in anyone's working tree —
// while CI sets the variable and collects the results.
const OutDirEnv = "ASTROGO_METROLOGY_OUT"

// Errors from persisting or reading a result document.
var (
	// ErrSchemaVersion marks a document written by a different version of
	// this package.
	ErrSchemaVersion = errors.New("metrology: unsupported schema version")

	// ErrWriteReport marks a failure to persist a result.
	ErrWriteReport = errors.New("metrology: cannot write report")

	// ErrDecodeReport marks a document that is not a result at all.
	ErrDecodeReport = errors.New("metrology: cannot decode result")
)

// Result is one suite's outcome, as written to disk and rendered into docs.
//
// The provenance fields exist so the document stays meaningful after
// everything around it has moved: an accuracy figure with no commit, no
// reference version and no date is a number with no claim attached.
type Result struct {
	SchemaVersion int    `json:"schema_version"`
	Suite         string `json:"suite"`
	Generated     string `json:"generated"`
	Commit        string `json:"commit"`
	GoVersion     string `json:"go_version"`
	Platform      string `json:"platform"`

	Status Status `json:"status"`

	// Reason is why a suite is NOT_VERIFIED, and empty otherwise.
	Reason string `json:"reason,omitempty"`

	Reference Reference `json:"reference"`
	Contract  Contract  `json:"contract"`
	Stats     Stats     `json:"stats"`
}

// commitOnce caches the revision lookup: every suite in a run stamps the same
// commit, and shelling out once per suite would be waste for a constant.
var commitOnce = sync.OnceValue(resolveCommit) //nolint:gochecknoglobals // a memoised constant, not mutable state

// resolveCommit finds the astrogo revision a result was produced at.
//
// Build info first, which is where it lives for a normal binary. Test binaries
// are not VCS-stamped, though — which is every caller of this package — so the
// fallback is git itself, the same way the corpus generator stamps its
// manifest.
//
// Best-effort by design: "unknown" is more honest than a guess, and a result
// is still worth recording without it. But it is worth trying for, because
// the commit is what makes a row's staleness visible — a figure confirmed
// before the module it describes was rewritten is not a validated figure, and
// a date alone does not say which code it describes.
func resolveCommit() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				return s.Value
			}
		}
	}

	out, err := exec.Command("git", "rev-parse", "HEAD").Output() //nolint:noctx // a fixed command with no user input
	if err != nil {
		return "unknown"
	}

	if rev := strings.TrimSpace(string(out)); rev != "" {
		return rev
	}

	return "unknown"
}

// buildStamp describes where a result came from.
func buildStamp() (commit, goVersion, platform string) {
	return commitOnce(), runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH
}

// newResult assembles a Result from a suite and an outcome.
func (s *Suite) newResult(status Status, reason string, stats Stats) Result {
	commit, goVersion, platform := buildStamp()

	return Result{
		SchemaVersion: SchemaVersion,
		Suite:         s.Name,
		// UTC and RFC3339, so results from different machines sort and
		// compare without anyone having to think about time zones.
		Generated: time.Now().UTC().Format(time.RFC3339),
		Commit:    commit,
		GoVersion: goVersion,
		Platform:  platform,
		Status:    status,
		Reason:    reason,
		Reference: s.Reference,
		Contract:  s.Contract,
		Stats:     stats,
	}
}

// maxReportedViolations bounds how many individual failures a suite prints
// before summarising the rest. A suite comparing thousands of points against
// a broken contract would otherwise bury its own summary in its own output.
const maxReportedViolations = 5

// Report publishes the suite: it logs the summary, fails tb for every sample
// outside the contract, writes the JSON document when [OutDirEnv] is set, and
// returns the statistics for a caller that wants to assert something further.
//
// The summary is logged whether or not the suite passed, which is the point
// of this package — see its doc comment — and is how the leap-second bug that
// prompted it was found.
func (s *Suite) Report(tb TB) Stats {
	tb.Helper()

	stats := s.Stats()

	status := StatusVerified
	if stats.Exceeding > 0 {
		status = StatusViolated
	}

	tb.Log(s.summary(stats, status))

	if stats.Rejected > 0 {
		tb.Errorf("%s: %d of %d samples were NaN or infinite and were dropped; "+
			"the statistics above describe only the %d that were not",
			s.Name, stats.Rejected, stats.Rejected+stats.N, stats.N)
	}

	reported := 0

	for _, sample := range s.samples {
		if abs(sample.Error) <= s.Contract.Max {
			continue
		}

		reported++

		if reported > maxReportedViolations {
			continue
		}

		tb.Errorf("%s exceeds contract\n  suite:     %s\n  context:   %s\n  measured:  %.6g %s\n  contract:  %.6g %s\n  because:   %s\n  source:    %s\n  reference: %s",
			sample.Label, s.Name, sample.Context,
			sample.Error, s.Contract.Units,
			s.Contract.Max, s.Contract.Units,
			s.Contract.Rationale, s.Contract.Source,
			s.Reference.Provenance())
	}

	if reported > maxReportedViolations {
		tb.Errorf("%s: %d further samples exceed the contract, not listed individually",
			s.Name, reported-maxReportedViolations)
	}

	s.write(tb, s.newResult(status, "", stats))

	return stats
}

// NotVerified records that one or more suites could not run, then skips tb.
//
// Call it instead of tb.Skip whenever a suite is abandoned for a reason
// outside astrogo — an unreachable service, missing reference data, a kernel
// that is not cached. The record is the whole point: a suite that simply
// vanishes from the report is indistinguishable from one that never existed,
// so an endpoint that dies permanently would leave the last good numbers on
// display with nothing to mark them stale.
//
// It takes the suites rather than being a method on one because a test
// function usually measures several quantities from the same fetched data,
// and all of them go unverified together when the fetch fails. Skipping is
// terminal for the goroutine, so a per-suite method could only ever record
// the first of them.
func NotVerified(tb TB, reason string, suites ...*Suite) {
	tb.Helper()

	names := make([]string, 0, len(suites))

	for _, s := range suites {
		s.write(tb, s.newResult(StatusNotVerified, reason, Stats{}))
		names = append(names, s.Name)
	}

	tb.Skipf("%s: NOT VERIFIED — %s", strings.Join(names, ", "), reason)
}

// summary is the human-readable block Report logs.
func (s *Suite) summary(stats Stats, status Status) string {
	var b strings.Builder

	u := s.Contract.Units

	fmt.Fprintf(&b, "\n── %s: %s ──\n", s.Name, status)
	fmt.Fprintf(&b, "  reference: %s\n", s.Reference.Provenance())
	fmt.Fprintf(&b, "  contract:  %s\n", s.Contract)
	fmt.Fprintf(&b, "  measured:  n=%d  p50=%.4g  p90=%.4g  p95=%.4g  p99=%.4g  max=%.4g %s\n",
		stats.N, stats.P50, stats.P90, stats.P95, stats.P99, stats.Max, u)
	fmt.Fprintf(&b, "             mean=%.4g  rms=%.4g  signed mean=%+.4g  range=[%+.4g, %+.4g] %s\n",
		stats.Mean, stats.RMS, stats.MeanSigned, stats.MinSigned, stats.MaxSigned, u)

	if stats.N > 0 {
		fmt.Fprintf(&b, "  worst:     %.4g %s at %s (%s)\n",
			stats.Max, u, stats.Worst.Label, stats.Worst.Context)
	}

	if stats.Exceeding > 0 {
		fmt.Fprintf(&b, "  exceeding: %d of %d\n", stats.Exceeding, stats.N)
	}

	if stats.Rejected > 0 {
		fmt.Fprintf(&b, "  rejected:  %d non-finite\n", stats.Rejected)
	}

	// Headroom says at a glance whether the contract is doing any work. A
	// factor near 1 means it is pinned to the measurement and cannot detect
	// a regression; a factor in the thousands means it cannot detect one
	// either, for the opposite reason. Both were live in this repository.
	if stats.N > 0 && stats.Max > 0 {
		fmt.Fprintf(&b, "  headroom:  contract is %.3gx the measured max\n", s.Contract.Max/stats.Max)
	}

	return b.String()
}

// write persists the result when OutDirEnv is set, and does nothing when it
// is not.
func (s *Suite) write(tb TB, res Result) {
	tb.Helper()

	dir := os.Getenv(OutDirEnv)
	if dir == "" {
		return
	}

	// 0o750/0o600 rather than the 0o755/0o644 this repository uses for its
	// user-cache directories: these are CI artifacts holding measurement
	// records, and nothing else needs to read them.

	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		tb.Errorf("%v: marshalling %s: %v", ErrWriteReport, s.Name, err)

		return
	}

	// gosec flags dir as tainted because it comes from the environment.
	// That is the design: the operator names their own output directory, and
	// a path they chose is not an attack on themselves. The only component
	// derived from program data is the file name, and FileName restricts it
	// to alphanumerics, dot, underscore and dash — no separators — so nothing
	// under this directory can be escaped by a suite name.
	//nolint:gosec // G703: operator-supplied directory by design; the suite-derived component is sanitised by FileName
	if err := os.MkdirAll(dir, 0o750); err != nil {
		tb.Errorf("%v: creating %s: %v", ErrWriteReport, dir, err)

		return
	}

	path := filepath.Join(dir, FileName(s.Name))
	//nolint:gosec // G703: see the note on MkdirAll above
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		tb.Errorf("%v: writing %s: %v", ErrWriteReport, path, err)
	}
}

// FileName is the document name a suite is written under: its own name with
// anything awkward for a filesystem replaced, plus a .json extension.
func FileName(suite string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, suite)

	return safe + ".json"
}

// ReadResult decodes a result document, refusing one written under a
// different schema rather than reinterpreting it.
func ReadResult(data []byte) (Result, error) {
	var res Result

	if err := json.Unmarshal(data, &res); err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrDecodeReport, err)
	}

	if res.SchemaVersion != SchemaVersion {
		return Result{}, fmt.Errorf("%w: document is version %d, this build reads version %d",
			ErrSchemaVersion, res.SchemaVersion, SchemaVersion)
	}

	return res, nil
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}

	return v
}
