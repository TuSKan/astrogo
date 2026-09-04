package lsk_test

import (
	"bufio"
	"context"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// knownAssignments is every DELTET keyword this package models.
//
// The kernel currently carries exactly these five and nothing else. That is a
// fact about naif0012.tls, not a guarantee about LSKs in general — SPICE text
// kernels can assign anything inside a \begindata block, and a future
// naif0013.tls could introduce a keyword.
var knownAssignments = map[string]bool{
	"DELTET/DELTA_T_A": true,
	"DELTET/K":         true,
	"DELTET/EB":        true,
	"DELTET/M":         true,
	"DELTET/DELTA_AT":  true,
}

// TestKernelHasNothingWeIgnore asserts that every assignment in the kernel's
// data block is one this package models, and that every constant it models was
// actually populated.
//
// # Why this exists
//
// Because "what else is in this file that we are not using?" should not be a
// question anyone answers by reading it. It was answered by reading it once,
// and reading it once is exactly how this package spent a release believing the
// LSK was a leap-second table: the periodic-term constants sat in the file the
// whole time, unparsed, while a doc comment asserted a leap-second kernel could
// not carry them.
//
// A parser that silently drops what it does not recognise cannot tell you what
// it dropped. This makes the kernel prove its own coverage instead, in both
// directions:
//
//   - Nothing in the file is ignored. An unrecognised assignment fails here
//     rather than being skipped, so a future kernel revision that adds a
//     keyword is a test failure and not a silent omission.
//   - Nothing we claim to read is missing. A constant left at zero means the
//     parse failed — the Fortran "1.657D-3" exponent form is the likely
//     culprit — and a zero K silently disables the periodic term rather than
//     erroring.
func TestKernelHasNothingWeIgnore(t *testing.T) {
	fetchCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	prov, err := jpl.NewProvider(fetchCtx, core.Planets, "de440s")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("setup failed: %v", err)
	}

	t.Cleanup(func() {
		if err := prov.Close(); err != nil {
			t.Errorf("close provider: %v", err)
		}
	})

	ctx := context.Background()

	bucket, prefix, err := remote.CacheDir(ctx, remote.NAIFLSK)
	testutil.AssertNoError(t, err)

	// ── Direction 1: nothing in the data block is unmodelled ────────────────
	raw, err := bucket.NewReader(ctx, prefix+"lsk/naif0012.tls", nil)
	if err != nil {
		t.Fatalf("open cached LSK: %v", err)
	}

	defer func() { _ = raw.Close() }()

	var (
		inData bool
		seen   int
	)

	scanner := bufio.NewScanner(raw)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch {
		case strings.HasPrefix(line, `\begindata`):
			inData = true

			continue
		case strings.HasPrefix(line, `\begintext`):
			inData = false

			continue
		}

		// Only a line that opens an assignment names a keyword; continuation
		// lines of the DELTA_AT block carry values alone.
		lhs, _, ok := strings.Cut(line, "=")
		if !inData || !ok || !strings.Contains(lhs, "/") {
			continue
		}

		key := strings.TrimSpace(lhs)
		seen++

		if !knownAssignments[key] {
			t.Errorf("the kernel assigns %q, which this package does not model.\n"+
				"  Either parse it or add it to knownAssignments with a note saying "+
				"why it is deliberately ignored — do not let the parser drop it "+
				"silently.", key)
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan the kernel: %v", err)
	}

	if seen != len(knownAssignments) {
		t.Errorf("found %d assignments in the data block, expected %d — the kernel's "+
			"contents changed", seen, len(knownAssignments))
	}

	// ── Direction 2: everything we model was actually populated ─────────────
	f, err := bucket.NewReader(ctx, prefix+"lsk/naif0012.tls", nil)
	if err != nil {
		t.Fatalf("reopen cached LSK: %v", err)
	}

	r, err := lsk.NewReader(f)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("close reader: %v", err)
		}
	})

	for _, c := range []struct {
		name string
		got  float64
	}{
		{"DELTA_T_A", r.DeltaTA},
		{"K", r.K},
		{"EB", r.EB},
		{"M0", r.M0},
		{"M1", r.M1},
	} {
		if c.got == 0 {
			t.Errorf("%s parsed as zero. A zero K or EB silently disables the "+
				"periodic term rather than failing, so this is checked explicitly; "+
				"the likely cause is the Fortran \"1.657D-3\" exponent form.", c.name)
		}
	}

	if len(r.DeltaAt) == 0 {
		t.Error("DELTA_AT parsed as empty")
	}

	t.Logf("%d assignments in the data block, all modelled; %d DELTA_AT entries",
		seen, len(r.DeltaAt))
}
