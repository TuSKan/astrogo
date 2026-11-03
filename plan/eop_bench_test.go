package plan

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"

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

	switch mode {
	case "zero":
		time.RegisterModel(time.ZeroModel{})
	case "table":
		table, err := time.ParseFinals2000A(strings.NewReader(testutil.FinalsText(benchEOPStart, benchEOPDays)))
		if err != nil {
			b.Fatal(err)
		}

		time.RegisterModel(table)
	default:
		b.Fatalf("unknown EOP mode %q", mode)
	}

	b.Cleanup(time.ResetEOP)

	if got := time.EOPSource(); got != "explicit" {
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
