//go:build integration

package workload_test

import (
	"math/rand/v2"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

var (
	// epoch is fixed: a benchmark's epoch is part of its subject, and one that
	// moved with the wall clock could not be compared with an earlier run.
	epoch = time.Date(2026, time.June, 1, 0, 0, 0, 0, time.LocationUTC)

	// sky is the solar system as a planner asks for it.
	sky = []core.ID{
		core.Sun, core.Moon, core.Mercury, core.Venus, core.Mars,
		core.Jupiter, core.Saturn, core.Uranus, core.Neptune,
	}

	// plutoAsMinorPlanet is Pluto's NAIF minor-planet ID (2000000 + 134340),
	// under which the built-in Pluto elements are registered so that a JPL
	// base's own Pluto barycenter cannot answer in their place.
	plutoAsMinorPlanet = core.ID(2134340)
)

// place is a function from provider, target and epoch to a state: the
// provider's own geometric State, or one of eph's light-time places.
type place = func(core.Provider, core.ID, time.Time) (core.State, error)

// measure runs op once outside the clock — the first call of anything pays
// for lazy loads that are not its cost — zeroes the counters, times op, and
// reports what one op cost in State calls and segment evaluations. Either
// counter may be nil.
func measure(b *testing.B, cp *countingProvider, f *readCounter, op func() error) {
	b.Helper()

	if err := op(); err != nil {
		b.Fatal(err)
	}

	if cp != nil {
		cp.calls.Store(0)
	}

	if f != nil {
		f.reads.Store(0)
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := op(); err != nil {
			b.Fatal(err)
		}
	}

	// After b.Loop returns, b.N is the number of iterations it ran.
	n := float64(b.N)
	if cp != nil {
		b.ReportMetric(float64(cp.calls.Load())/n, "states/op")
	}

	if f != nil {
		b.ReportMetric(float64(f.evaluations())/n, "evals/op")
	}
}

// keplerPluto is a Kepler provider propagating the built-in Pluto elements
// over base (the internal SOFA source when nil).
func keplerPluto(b *testing.B, base core.Provider) core.Provider {
	b.Helper()

	var opts []kepler.Option
	if base != nil {
		opts = append(opts, kepler.WithBase(base))
	}

	p := kepler.New(opts...)
	if err := p.Register(plutoAsMinorPlanet, kepler.PlutoElements()); err != nil {
		b.Fatalf("register Pluto: %v", err)
	}

	return p
}

func stateOf(p core.Provider, id core.ID) func() error {
	return func() error {
		_, err := p.State(id, epoch)

		return err //nolint:wrapcheck // the provider's error names the body
	}
}

// BenchmarkOneBodyOneEpoch (B1): one body at one epoch, the unit every other
// workload is made of.
func BenchmarkOneBodyOneEpoch(b *testing.B) {
	for _, be := range backends {
		for _, id := range []core.ID{core.Mars, core.Moon} {
			b.Run("jpl-"+be.name+"/"+id.String(), func(b *testing.B) {
				p, f := be.open(b)
				measure(b, nil, f, stateOf(p, id))
			})
		}
	}

	for _, id := range []core.ID{core.Mars, core.Moon} {
		b.Run("default/"+id.String(), func(b *testing.B) {
			measure(b, nil, nil, stateOf(eph.Default(), id))
		})
	}

	b.Run("kepler-sofa/Pluto", func(b *testing.B) {
		measure(b, nil, nil, stateOf(keplerPluto(b, nil), plutoAsMinorPlanet))
	})

	b.Run("kepler-jpl/Pluto", func(b *testing.B) {
		p, f := openCounted(b)
		measure(b, nil, f, stateOf(keplerPluto(b, p), plutoAsMinorPlanet))
	})
}

// BenchmarkSkyOneEpoch (B2): many bodies at one epoch, a planner's sky at one
// instant. Every State is geocentric, so each re-derives the Earth.
func BenchmarkSkyOneEpoch(b *testing.B) {
	sweep := func(cp *countingProvider, at place) func() error {
		return func() error {
			for _, id := range sky {
				if _, err := at(cp, id, epoch); err != nil {
					return err
				}
			}

			return nil
		}
	}

	for _, be := range backends {
		b.Run("geometric/jpl-"+be.name, func(b *testing.B) {
			p, f := be.open(b)
			cp := &countingProvider{Provider: p}
			measure(b, cp, f, sweep(cp, core.Provider.State))
		})
	}

	b.Run("geometric/default", func(b *testing.B) {
		cp := &countingProvider{Provider: eph.Default()}
		measure(b, cp, nil, sweep(cp, core.Provider.State))
	})

	b.Run("apparent/jpl-bytes", func(b *testing.B) {
		p, f := openCounted(b)
		cp := &countingProvider{Provider: p}
		measure(b, cp, f, sweep(cp, eph.ApparentState))
	})

	b.Run("apparent/default", func(b *testing.B) {
		cp := &countingProvider{Provider: eph.Default()}
		measure(b, cp, nil, sweep(cp, eph.ApparentState))
	})
}

// BenchmarkOneBodyManyEpochs (B3): one body over many epochs — a track, a
// search, a light curve. Shuffling the epochs defeats whatever locality the
// reader's chunk cache would otherwise find; the dense night stays within a
// record or two, the decade crosses hundreds.
func BenchmarkOneBodyManyEpochs(b *testing.B) {
	const n = 1000

	for _, span := range []struct {
		name string
		step unit.Duration
	}{
		{"night-1min", unit.Minutes(1)},
		{"decade-4d", unit.Days(4)},
	} {
		for _, shuffled := range []bool{false, true} {
			order := "sequential"
			times := make([]time.Time, n)

			for i := range times {
				times[i] = epoch.Add(span.step * unit.Duration(i))
			}

			if shuffled {
				order = "shuffled"

				rand.New(rand.NewPCG(1, 2)).Shuffle(n, func(i, j int) { times[i], times[j] = times[j], times[i] })
			}

			for _, be := range backends {
				b.Run(span.name+"/"+order+"/jpl-"+be.name, func(b *testing.B) {
					p, f := be.open(b)
					measure(b, nil, f, func() error {
						for _, t := range times {
							if _, err := p.State(core.Moon, t); err != nil {
								return err //nolint:wrapcheck // the provider's error names the body
							}
						}

						return nil
					})
					b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/state")
				})
			}
		}
	}
}

// BenchmarkLightTime (B4): the astrometric and apparent places iterate the
// target back to its emission time, then correct for the observer.
func BenchmarkLightTime(b *testing.B) {
	for _, pl := range []struct {
		name string
		at   place
	}{
		{"astrometric", eph.AstrometricState},
		{"apparent", eph.ApparentState},
	} {
		for _, id := range []core.ID{core.Moon, core.Mars, core.Neptune} {
			b.Run(pl.name+"/jpl-bytes/"+id.String(), func(b *testing.B) {
				p, f := openCounted(b)
				cp := &countingProvider{Provider: p}
				measure(b, cp, f, func() error { _, err := pl.at(cp, id, epoch); return err })
			})

			b.Run(pl.name+"/default/"+id.String(), func(b *testing.B) {
				cp := &countingProvider{Provider: eph.Default()}
				measure(b, cp, nil, func() error { _, err := pl.at(cp, id, epoch); return err })
			})
		}
	}
}

// BenchmarkMoonAboutItsPlanet (B5): a moon relative to its planet. The
// provider answers only geocentric states, so "provider" differences two of
// them, each carried to the solar-system barycenter and back to the Earth;
// the kernel already holds both bodies relative to the planet's barycenter,
// which "native" evaluates directly.
func BenchmarkMoonAboutItsPlanet(b *testing.B) {
	for _, c := range []struct {
		name         string
		moon, planet int32
		kernel       string // a satellite kernel beyond de440s, or ""
	}{
		{"Moon-Earth", 301, 399, ""},
		{"Triton-Neptune", 801, 899, "satellites/nep097.bsp"},
	} {
		open := func(b *testing.B) (*jpl.Provider, *readCounter) {
			b.Helper()

			p, f := openCounted(b)
			if c.kernel != "" {
				openCachedKernel(b, p, f, c.kernel)
			}

			return p, f
		}

		b.Run(c.name+"/provider", func(b *testing.B) {
			p, f := open(b)
			measure(b, nil, f, func() error {
				_, err := relativeViaProvider(p, core.ID(c.moon), core.ID(c.planet))

				return err
			})
		})

		b.Run(c.name+"/native", func(b *testing.B) {
			p, f := open(b)
			measure(b, nil, f, func() error {
				_, err := relativeNative(p, c.moon, c.planet, epoch)

				return err
			})
		})
	}
}

// relativeViaProvider is moon minus planet from two geocentric states, in AU:
// what a caller has to write today.
func relativeViaProvider(p core.Provider, moon, planet core.ID) (vector.Vec3, error) {
	m, err := p.State(moon, epoch)
	if err != nil {
		return vector.Vec3{}, err //nolint:wrapcheck // the provider's error names the body
	}

	pl, err := p.State(planet, epoch)
	if err != nil {
		return vector.Vec3{}, err //nolint:wrapcheck // the provider's error names the body
	}

	return m.Pos.Sub(pl.Pos), nil
}

// relativeNative is moon minus planet from the two segments that share the
// planet's barycenter as their center, in kilometers: the evaluation a
// target-and-center API would make.
func relativeNative(p *jpl.Provider, moon, planet int32, t time.Time) (vector.Vec3, error) {
	et := lsk.UTCToET(t, p.LSK)

	m, err := evaluate(p, moon, et)
	if err != nil {
		return vector.Vec3{}, err
	}

	pl, err := evaluate(p, planet, et)
	if err != nil {
		return vector.Vec3{}, err
	}

	return m.Sub(pl), nil
}

// evaluate is the position of target at et from its own segment's center, in
// kilometers.
func evaluate(p *jpl.Provider, target int32, et float64) (vector.Vec3, error) {
	ref, err := p.FindSegment(target, et)
	if err != nil {
		return vector.Vec3{}, err //nolint:wrapcheck // FindSegment's error names the target
	}

	k := p.Kernels[ref.KernelIndex]

	pos, _, err := spk.EvaluateSegment(&k.Segments[ref.SegmentIndex], k.Reader, et)

	return pos, err //nolint:wrapcheck // the reader's error describes the segment
}

// barcelona is the site the plan examples use.
func barcelona(b *testing.B) *coord.Geodetic {
	b.Helper()

	g, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), unit.Meters(0))
	if err != nil {
		b.Fatalf("site: %v", err)
	}

	return g
}

// BenchmarkSkyFromSite (B6): many targets at one site and time, a planner's
// observed sky, with one Context shared across every target as the scheduler
// shares it.
func BenchmarkSkyFromSite(b *testing.B) {
	run := func(b *testing.B, p core.Provider, f *readCounter) {
		b.Helper()

		cp := &countingProvider{Provider: p}
		ctx := coord.NewContext(epoch, barcelona(b), atmosphere.AtAltitude(0))

		bodies := make([]*plan.Planet, len(sky))
		for i, id := range sky {
			bodies[i] = plan.NewPlanet(id.String(), id, cp)
		}

		measure(b, cp, f, func() error {
			for _, body := range bodies {
				v, err := body.GeocentricVec(epoch)
				if err != nil {
					return err //nolint:wrapcheck // GeocentricVec names the planet
				}

				_ = ctx.GeocentricToObserved(v)
			}

			return nil
		})
	}

	b.Run("jpl-bytes", func(b *testing.B) {
		p, f := openCounted(b)
		run(b, p, f)
	})

	b.Run("jpl-disk", func(b *testing.B) {
		p, f := openDisk(b)
		run(b, p, f)
	})

	b.Run("default", func(b *testing.B) {
		run(b, eph.Default(), nil)
	})
}

// BenchmarkEventSearch (B7): plan's root finders call the provider many times
// per event, and that count is what a faster State would divide.
func BenchmarkEventSearch(b *testing.B) {
	site, err := plan.NewSite("Barcelona", barcelona(b))
	if err != nil {
		b.Fatalf("site: %v", err)
	}

	searches := []struct {
		name string
		run  func(p core.Provider) error
	}{
		{"oppositions-1y-Mars", func(p core.Provider) error {
			_, err := plan.Oppositions(epoch, epoch.Add(unit.Days(365)), plan.NewMars(p), plan.NewSun(p))

			return err //nolint:wrapcheck // the benchmark reports it under the search's name
		}},
		{"day-events-Moon", func(p core.Provider) error {
			_, _, _, err := plan.DayEvents(epoch, time.LocationUTC, plan.NewMoon(p), site)

			return err //nolint:wrapcheck // the benchmark reports it under the search's name
		}},
		{"day-events-Mars", func(p core.Provider) error {
			_, _, _, err := plan.DayEvents(epoch, time.LocationUTC, plan.NewMars(p), site)

			return err //nolint:wrapcheck // the benchmark reports it under the search's name
		}},
	}

	for _, s := range searches {
		b.Run(s.name+"/jpl-bytes", func(b *testing.B) {
			p, f := openCounted(b)
			cp := &countingProvider{Provider: p}
			measure(b, cp, f, func() error { return s.run(cp) })
		})

		b.Run(s.name+"/default", func(b *testing.B) {
			cp := &countingProvider{Provider: eph.Default()}
			measure(b, cp, nil, func() error { return s.run(cp) })
		})
	}
}

// The ISS element set of 2008-09-20 that the satellite package's own tests
// use.
const (
	issLine1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
	issLine2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
)

// BenchmarkSatellite (B8): SGP4 alone, then the GCRS state the provider
// returns (SGP4 and the TEME-to-GCRS rotation), then a pass sampled each
// minute.
func BenchmarkSatellite(b *testing.B) {
	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		b.Fatalf("TLE: %v", err)
	}

	at := time.Date(2008, time.September, 20, 12, 0, 0, 0, time.LocationUTC)

	b.Run("sgp4", func(b *testing.B) {
		measure(b, nil, nil, func() error {
			_, _, err := sat.Propagator().AtTime(at)

			return err //nolint:wrapcheck // SGP4's error is the one to report
		})
	})

	b.Run("state", func(b *testing.B) {
		measure(b, nil, nil, func() error {
			_, err := sat.State(0, at)

			return err //nolint:wrapcheck // the provider's error is the one to report
		})
	})

	b.Run("pass-6h-1min", func(b *testing.B) {
		times := make([]time.Time, 360)
		for i := range times {
			times[i] = at.Add(unit.Minutes(float64(i)))
		}

		measure(b, nil, nil, func() error {
			for _, t := range times {
				if _, err := sat.State(0, t); err != nil {
					return err //nolint:wrapcheck // the provider's error is the one to report
				}
			}

			return nil
		})
	})
}

// BenchmarkConcurrentReaders (B9): one provider shared by every goroutine,
// each asking for the Moon at its own random epochs across a decade — what a
// parallel scheduler and a web service both look like.
func BenchmarkConcurrentReaders(b *testing.B) {
	for _, be := range backends {
		b.Run("jpl-"+be.name, func(b *testing.B) {
			p, _ := be.open(b)

			var seed atomic.Uint64

			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				r := rand.New(rand.NewPCG(seed.Add(1), 0))

				for pb.Next() {
					t := epoch.Add(unit.Days(3650 * r.Float64()))
					if _, err := p.State(core.Moon, t); err != nil {
						b.Error(err)

						return
					}
				}
			})
		})
	}
}
