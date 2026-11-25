package plan

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/epoch"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// The benchmarks in this package run with the Earth orientation the process
// holds, which is none: nothing here registers a loader, and the library warns
// once that it is using zero DUT1 and polar motion. A real planning session has
// the data, and pays for reading it at every Context it builds. These run a
// scheduler, the workload that builds the most Contexts, with the orientation
// absent and present.

// benchEOPStart and benchEOPDays place a synthetic table over the benchmark
// epochs: MJD 59900 for 1500 days, to mid-2027.
const (
	benchEOPStart = 59900.0
	benchEOPDays  = 1500
)

var benchEOPModes = []string{"zero", "table"}

// pinEOP is the process-wide Earth orientation for the benchmark, restored to
// the library default when it ends. RegisterModel makes the choice
// authoritative, so a cache on disk can not replace it.
func pinEOP(b *testing.B, mode string) {
	b.Helper()
	keepEOP(b)

	switch mode {
	case "zero":
		epoch.RegisterModel(epoch.ZeroModel{})
	case "table":
		table, err := epoch.ParseFinals2000A(strings.NewReader(testutil.FinalsText(benchEOPStart, benchEOPDays)))
		if err != nil {
			b.Fatal(err)
		}

		epoch.RegisterModel(table)
	default:
		b.Fatalf("unknown EOP mode %q", mode)
	}

	if got := epoch.EOPSource(); got != "explicit" {
		b.Fatalf("EOP source = %q after pinning %q, want explicit: the benchmark would time ambient data", got, mode)
	}
}

// BenchmarkGreedyStrategy50EOP is BenchmarkGreedyStrategy_50 with Earth
// orientation absent and present.
func BenchmarkGreedyStrategy50EOP(b *testing.B) {
	for _, mode := range benchEOPModes {
		b.Run(mode, func(b *testing.B) {
			pinEOP(b, mode)
			benchScheduler(b, 50, &GreedyStrategy{})
		})
	}
}

// keepEOP puts back, when tb ends, the Earth orientation it found: the zero
// model this package's TestMain pins, chosen explicitly, rather than
// epoch.ResetEOP's default, which would let the lazy load replace it for every
// benchmark after this one.
func keepEOP(tb testing.TB) {
	tb.Helper()

	model, explicit := epoch.GetModel(), epoch.EOPSource() == "explicit"

	tb.Cleanup(func() {
		if explicit {
			epoch.RegisterModel(model)

			return
		}

		epoch.ResetEOP()
	})
}
