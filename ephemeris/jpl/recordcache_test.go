package jpl_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// TestRecordCacheReproducesDe440s: every segment of de440s, evaluated through
// the provider's own Reader — which caches each segment's directory and last
// record (#802) — against an uncached Reader over the same file, over a year
// of hourly epochs in order and then shuffled, so every record boundary of
// 2026 is crossed both ways and the cache sees its hits and its misses. The
// arithmetic is the same, so the answers must be the same to the last bit.
func TestRecordCacheReproducesDe440s(t *testing.T) {
	t.Parallel()

	p := mustPlanetProvider(t)

	t.Cleanup(func() { _ = p.Close() })

	const (
		start = 8.3e8 // ET seconds: late 2026
		hours = 365 * 24
	)

	ets := make([]float64, 0, 2*hours)
	for i := range hours {
		ets = append(ets, start+3600*float64(i))
	}

	shuffled := slices.Clone(ets)
	rand.New(rand.NewPCG(1, 2)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	ets = append(ets, shuffled...)

	checked := 0

	for _, k := range p.Kernels {
		uncached := &spk.Reader{F: k.Reader.F, FileRec: k.Reader.FileRec}

		for i := range k.Segments {
			seg := &k.Segments[i]

			for _, et := range ets {
				if et < seg.StartET || et > seg.EndET {
					continue
				}

				wantP, wantV, wantErr := spk.EvaluateSegment(seg, uncached, et)
				gotP, gotV, gotErr := spk.EvaluateSegment(seg, k.Reader, et)

				if wantErr != nil || gotErr != nil {
					t.Fatalf("target %d at ET %v: uncached err %v, cached err %v", seg.Target, et, wantErr, gotErr)
				}

				if !bitIdentical(gotP, wantP) || !bitIdentical(gotV, wantV) {
					t.Fatalf("target %d at ET %v: cached %+v %+v, uncached %+v %+v", seg.Target, et, gotP, gotV, wantP, wantV)
				}

				checked++
			}
		}
	}

	if checked == 0 {
		t.Fatal("de440s had no segment covering 2026")
	}
}

// TestStateFromCachedRecordsAllocatesNothing is an allocation contract: once
// the records a State needs are cached (#802), answering it reads nothing,
// so it has nothing to allocate — not for a body whose chain is one segment
// (Mars) nor two (the Moon, through the Earth-Moon barycenter). Before #802 a
// geocentric Moon allocated 20 times.
//
// Not parallel: testing.AllocsPerRun refuses to run in a parallel test.
func TestStateFromCachedRecordsAllocatesNothing(t *testing.T) {
	p := mustPlanetProvider(t)

	t.Cleanup(func() { _ = p.Close() })

	at := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, id := range []core.ID{core.Sun, core.Moon, core.Mercury, core.MarsBarycenter, core.NeptuneBarycenter} {
		if _, err := eph.At(p, at).Geometric(id, core.Earth); err != nil {
			t.Fatalf("%v: %v", id, err)
		}

		if got := testing.AllocsPerRun(100, func() { _, _ = eph.At(p, at).Geometric(id, core.Earth) }); got != 0 {
			t.Errorf("%v: State from cached records allocates %v times, want 0", id, got)
		}
	}
}

func bitIdentical(a, b vector.Vec3) bool {
	return math.Float64bits(a.X) == math.Float64bits(b.X) &&
		math.Float64bits(a.Y) == math.Float64bits(b.Y) &&
		math.Float64bits(a.Z) == math.Float64bits(b.Z)
}
