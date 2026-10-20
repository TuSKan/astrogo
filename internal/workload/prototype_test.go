//go:build integration

package workload_test

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// A prototype of the two internals every architecture option in
// docs/ephemeris.md shares, built here so that their effect is measured
// rather than projected: a decoded-record cache per segment, and an instant
// that evaluates each barycentric chain once per epoch. It is not production
// code and handles only what de440s needs — one type 2 segment per target.
//
// Its arithmetic is the production arithmetic in the production order, which
// TestPrototypeReproducesTheProvider holds it to bit for bit; only the work
// around the arithmetic differs.

var (
	errProtoSegment = errors.New("prototype: unsupported segment")
	errProtoEpoch   = errors.New("prototype: epoch outside the segment's records")
)

// protoRecord is one decoded Chebyshev record. It is immutable once
// published, so readers share it without a lock.
type protoRecord struct {
	idx      int32
	mid, rad float64
	n        int       // coefficients per axis
	c        []float64 // x, y, z coefficients, n each
}

// protoSegment is a type 2 segment with its directory read once and its last
// record kept decoded.
type protoSegment struct {
	s            *spk.Segment
	r            *spk.Reader
	init, intlen float64
	rsize, nrec  int32
	cur          atomic.Pointer[protoRecord]
}

func newProtoSegment(s *spk.Segment, r *spk.Reader) (*protoSegment, error) {
	if s.Type != 2 {
		return nil, fmt.Errorf("%w: type %d", errProtoSegment, s.Type)
	}

	meta, err := r.ReadDoubles(s.EndAddr-3, s.EndAddr)
	if err != nil {
		return nil, fmt.Errorf("prototype: directory: %w", err)
	}

	return &protoSegment{s: s, r: r, init: meta[0], intlen: meta[1], rsize: int32(meta[2]), nrec: int32(meta[3])}, nil
}

func (ps *protoSegment) record(et float64) (*protoRecord, error) {
	pos := math.Floor((et - ps.init) / ps.intlen)
	if !(pos >= 0 && pos <= float64(ps.nrec)) {
		return nil, fmt.Errorf("%w: ET %v", errProtoEpoch, et)
	}

	idx := min(int32(pos), ps.nrec-1)

	if rec := ps.cur.Load(); rec != nil && rec.idx == idx {
		return rec, nil
	}

	start := ps.s.StartAddr + idx*ps.rsize

	words, err := ps.r.ReadDoubles(start, start+ps.rsize-1)
	if err != nil {
		return nil, fmt.Errorf("prototype: record: %w", err)
	}

	rec := &protoRecord{idx: idx, mid: words[0], rad: words[1], n: int((ps.rsize - 2) / 3), c: words[2:]}
	ps.cur.Store(rec)

	return rec, nil
}

// eval is the segment's state at et in km and km/s; with deriv false the
// velocity is not computed.
func (ps *protoSegment) eval(et float64, deriv bool) (pos, vel vector.Vec3, err error) {
	rec, err := ps.record(et)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	tau := (et - rec.mid) / rec.rad
	n := rec.n

	pos.X, vel.X = spk.EvalChebyshev(rec.c[:n], tau, rec.rad, deriv)
	pos.Y, vel.Y = spk.EvalChebyshev(rec.c[n:2*n], tau, rec.rad, deriv)
	pos.Z, vel.Z = spk.EvalChebyshev(rec.c[2*n:3*n], tau, rec.rad, deriv)

	return pos, vel, nil
}

// protoEphemeris is de440s's segments by target.
type protoEphemeris struct {
	segs map[int32]*protoSegment
	lsk  *lsk.Reader
}

func newProtoEphemeris(tb testing.TB, p *jpl.Provider) *protoEphemeris {
	tb.Helper()

	e := &protoEphemeris{segs: map[int32]*protoSegment{}, lsk: p.LSK}

	for _, k := range p.Kernels {
		for i := range k.Segments {
			s := &k.Segments[i]
			if _, dup := e.segs[s.Target]; dup {
				tb.Fatalf("prototype: target %d has more than one segment", s.Target)
			}

			ps, err := newProtoSegment(s, k.Reader)
			if err != nil {
				tb.Fatalf("target %d: %v", s.Target, err)
			}

			e.segs[s.Target] = ps
		}
	}

	return e
}

// protoInstant evaluates barycentric chains at one epoch, each body once.
type protoInstant struct {
	eph   *protoEphemeris
	et    float64
	deriv bool
	memo  bool

	// The memo: a body's barycentric state, found by linear search — a sky
	// holds a dozen bodies, and a map would allocate per instant.
	ids      []int32
	pos, vel []vector.Vec3
}

func (in *protoInstant) reset(et float64) {
	in.et = et
	in.ids, in.pos, in.vel = in.ids[:0], in.pos[:0], in.vel[:0]
}

// barycentric sums id's chain up to the barycenter in the production order:
// the body's own segment first, then each center's in turn.
func (in *protoInstant) barycentric(id int32) (pos, vel vector.Vec3, err error) {
	if in.memo {
		for i, have := range in.ids {
			if have == id {
				return in.pos[i], in.vel[i], nil
			}
		}
	}

	for cur := id; cur != 0; {
		ps, ok := in.eph.segs[cur]
		if !ok {
			return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: no segment for %d", errProtoSegment, cur)
		}

		p, v, err := ps.eval(in.et, in.deriv)
		if err != nil {
			return vector.Vec3{}, vector.Vec3{}, err
		}

		pos, vel = pos.Add(p), vel.Add(v)
		cur = ps.s.Center
	}

	if in.memo {
		in.ids, in.pos, in.vel = append(in.ids, id), append(in.pos, pos), append(in.vel, vel)
	}

	return pos, vel, nil
}

// geocentric is jpl.Provider.State's arithmetic: the body's barycentric
// state less the Earth's, in AU and AU/day.
func (in *protoInstant) geocentric(id int32) (core.State, error) {
	t, tv, err := in.barycentric(id)
	if err != nil {
		return core.State{}, err
	}

	e, ev, err := in.barycentric(399)
	if err != nil {
		return core.State{}, err
	}

	rel, relV := t.Sub(e), tv.Sub(ev)

	return core.State{
		Pos: vector.Vec3{X: rel.X / kmPerAU, Y: rel.Y / kmPerAU, Z: rel.Z / kmPerAU},
		Vel: vector.Vec3{X: relV.X * 86400 / kmPerAU, Y: relV.Y * 86400 / kmPerAU, Z: relV.Z * 86400 / kmPerAU},
	}, nil
}

// skyNAIF is the sky as NAIF codes, in the order of sky.
var skyNAIF = []int32{10, 301, 199, 299, 4, 5, 6, 7, 8}

// TestPrototypeReproducesTheProvider: the prototype changes the work around
// the arithmetic and nothing else, so every state must be the provider's to
// the last bit — over a year of hourly epochs, which crosses thousands of
// records and exercises the cache's miss path as well as its hits.
func TestPrototypeReproducesTheProvider(t *testing.T) {
	p, _ := openCounted(t)
	eph := newProtoEphemeris(t, p)
	in := &protoInstant{eph: eph, deriv: true, memo: true}

	for h := range 365 * 24 {
		at := epoch.Add(unit.Hours(float64(h)))
		in.reset(lsk.UTCToET(at, p.LSK))

		for i, id := range sky {
			want, err := p.State(id, at)
			if err != nil {
				t.Fatalf("%v at %v: %v", id, at, err)
			}

			got, err := in.geocentric(skyNAIF[i])
			if err != nil {
				t.Fatalf("prototype %v at %v: %v", id, at, err)
			}

			if got.Pos != want.Pos || got.Vel != want.Vel {
				t.Fatalf("%v at %v: prototype %+v, provider %+v", id, at, got, want)
			}
		}
	}
}

// BenchmarkPrototypeSky (B2 again): the geometric sky at one epoch, by the
// provider as it is and by the prototype with each internal switched on in
// turn. The disk backend is the one a caller gets.
func BenchmarkPrototypeSky(b *testing.B) {
	for _, be := range []backend{backends[0], backends[2]} {
		p, _ := be.open(b)
		eph := newProtoEphemeris(b, p)

		for _, v := range []struct {
			name        string
			memo, deriv bool
		}{
			{"record-cache", false, true},
			{"record-cache+instant", true, true},
			{"record-cache+instant+position-only", true, false},
		} {
			b.Run("jpl-"+be.name+"/"+v.name, func(b *testing.B) {
				in := &protoInstant{eph: eph, deriv: v.deriv, memo: v.memo}

				measure(b, nil, nil, func() error {
					in.reset(lsk.UTCToET(epoch, p.LSK))

					for _, id := range skyNAIF {
						if _, err := in.geocentric(id); err != nil {
							return err
						}
					}

					return nil
				})
			})
		}
	}
}

// BenchmarkPrototypeTrack (B3 again): the Moon over 1000 epochs, one minute
// apart and four days apart, where consecutive epochs share a record and
// where they never do.
func BenchmarkPrototypeTrack(b *testing.B) {
	const n = 1000

	p, _ := openDisk(b)
	eph := newProtoEphemeris(b, p)

	for _, span := range []struct {
		name string
		step unit.Duration
	}{
		{"night-1min", unit.Minutes(1)},
		{"decade-4d", unit.Days(4)},
	} {
		times := make([]time.Time, n)
		for i := range times {
			times[i] = epoch.Add(span.step * unit.Duration(i))
		}

		b.Run(span.name+"/jpl-disk/prototype", func(b *testing.B) {
			in := &protoInstant{eph: eph, deriv: true, memo: true}

			measure(b, nil, nil, func() error {
				for _, at := range times {
					in.reset(lsk.UTCToET(at, p.LSK))

					if _, err := in.geocentric(301); err != nil {
						return err
					}
				}

				return nil
			})
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/state")
		})
	}
}

// BenchmarkPrototypeConcurrent (B9 again): the shared, lock-free record cache
// under sixteen readers, at one epoch (every read a hit) and at random epochs
// across a decade (nearly every read a miss, which still goes through the
// chunk cache's lock).
func BenchmarkPrototypeConcurrent(b *testing.B) {
	p, _ := openDisk(b)
	eph := newProtoEphemeris(b, p)

	for _, random := range []bool{false, true} {
		name := "one-epoch"
		if random {
			name = "random-decade"
		}

		b.Run(name+"/jpl-disk/prototype", func(b *testing.B) {
			var seed atomic.Uint64

			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				r := rand.New(rand.NewPCG(seed.Add(1), 0))
				in := &protoInstant{eph: eph, deriv: true, memo: true}

				for pb.Next() {
					at := epoch
					if random {
						at = epoch.Add(unit.Days(3650 * r.Float64()))
					}

					in.reset(lsk.UTCToET(at, p.LSK))

					if _, err := in.geocentric(301); err != nil {
						b.Error(err)

						return
					}
				}
			})
		})
	}
}
