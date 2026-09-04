package ephemeris_test

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
)

// metresInAU converts the tolerance below into the AU that State reports in.
const metresInAU = 1.0 / 149597870700.0

// scaleTolerance is one metre.
//
// Not a physical accuracy budget — the correct answer here is the *same*
// answer, and the only permitted difference is the floating-point cost of
// converting a Julian Date between scales and back. Measured, that cost is
// 7e-8 m at worst across these providers, so one metre leaves seven orders of
// headroom for a platform whose FMA rounding differs while staying five orders
// below the smallest defect this test exists to catch (40 arcsec of lunar
// motion is ~74 km; the SGP4 defect was 530 km).
const scaleTolerance = 1.0 * metresInAU

// TestProviderStateIsScaleInvariant asserts that a provider returns the same
// state for one physical instant however the caller labels its scale.
//
// # Why this test exists
//
// time.Time is scale-aware by design. A provider that reads its Julian Date
// parts without normalising does not get a slightly different answer — it
// silently reinterprets the caller's instant as a different one.
//
// Three providers had three different contracts for the same input type:
//
//   - SOFA: correct, and always was.
//   - JPL: anything that was not already TDB was treated as UTC, so a TT input
//     had leap seconds plus 32.184 s added on top of an offset it already
//     carried. 69.184 s late — about 40 arcsec of lunar motion.
//   - SGP4: everything was treated as UTC. Also 69.184 s late for a TT input,
//     which at the ISS's 7.66 km/s is 530 km, and 283 km for TAI. A pass
//     prediction wrong by roughly a minute.
//
// Neither defect was a wrong formula. Both were a missing conversion at the
// boundary, and nothing in the suite asked the question that finds them: each
// provider was validated against its own external reference, and no test
// compared a provider against itself under a relabelled input.
//
// # Why the tolerance is not an accuracy budget
//
// Every other numerical test here compares against an external reference and
// tolerates real physical disagreement. This one compares a provider with
// itself, so the expected difference is zero and the tolerance exists only to
// absorb float round-trip noise. See scaleTolerance.
//
// # Coverage
//
// SOFA and SGP4 run here because they need no kernel. The JPL provider needs
// one, so its case lives in ephemeris/jpl/validation under the validation tag —
// it is the same assertion against the same table.
func TestProviderStateIsScaleInvariant(t *testing.T) {
	t.Parallel()

	// A fixed epoch, not the wall clock: a test pinned to "now" drifts out of
	// IERS coverage and fails on some future Tuesday with no code change.
	utc := time.Date(2026, time.April, 20, 3, 0, 0, 0, time.LocationUTC)

	sofa := eph.Default()

	// The repo's own ISS elements, so this test does not depend on the network
	// and does not rot when a fresher TLE appears.
	sat, err := satellite.NewFromTLE("ISS",
		"1 25544U 98067A   26109.48995873  .00010082  00000-0  19194-3 0  9999",
		"2 25544  51.6329 230.6068 0006631 325.6576  34.3983 15.48833250562656")
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	providers := []struct {
		name string
		prov core.Provider
		ids  map[string]core.ID
	}{
		{
			name: "sofa",
			prov: sofa,
			ids: map[string]core.ID{
				"Sun":     eph.Sun,
				"Moon":    eph.Moon,
				"Mars":    eph.Mars,
				"Jupiter": eph.Jupiter,
			},
		},
		{
			// SGP4 is the fastest-moving body the library propagates, so it is
			// the most sensitive probe of an epoch slip: 7.66 km/s turns one
			// second of error into 7.66 km.
			name: "sgp4",
			prov: sat,
			ids:  map[string]core.ID{"ISS": core.ID(0)},
		},
	}

	scales := []struct {
		label string
		at    func(time.Time) time.Time
	}{
		{"UTC", func(t time.Time) time.Time { return t.UTC() }},
		{"TAI", func(t time.Time) time.Time { return t.TAI() }},
		{"TT", func(t time.Time) time.Time { return t.TT() }},
		{"TDB", func(t time.Time) time.Time { return t.TDB() }},
	}

	for _, p := range providers {
		for body, id := range p.ids {
			t.Run(p.name+"/"+body, func(t *testing.T) {
				t.Parallel()

				base, err := p.prov.State(id, utc)
				if err != nil {
					t.Fatalf("State at the reference epoch: %v", err)
				}

				for _, s := range scales {
					got, err := p.prov.State(id, s.at(utc))
					if err != nil {
						t.Fatalf("State with a %s-labelled instant: %v", s.label, err)
					}

					if d := got.Pos.Sub(base.Pos).Norm(); d > scaleTolerance {
						t.Errorf("position moved %.6g AU (%.4g km) when the same instant "+
							"was labelled %s.\n  The provider is reading the caller's "+
							"scale as its own — normalise at the entry point "+
							"(t.UTC()/t.TDB()) rather than reading JDParts raw.",
							d, d/metresInAU/1e3, s.label)
					}

					if d := got.Vel.Sub(base.Vel).Norm(); d > scaleTolerance {
						t.Errorf("velocity moved %.6g AU/day when the same instant was "+
							"labelled %s", d, s.label)
					}
				}
			})
		}
	}
}
