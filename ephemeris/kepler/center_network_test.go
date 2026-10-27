//go:build network

package kepler_test

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// centerBodies are the small bodies the center choice was measured on (#803):
// every class from the near-Earth asteroids to Sedna, and comets from Encke to
// Hale-Bopp, as Horizons COMMAND strings.
var centerBodies = []struct{ name, command string }{
	{"Eros", "433;"}, {"Apophis", "99942;"}, {"Phaethon", "3200;"},
	{"Ceres", "1;"}, {"Vesta", "4;"}, {"Hygiea", "10;"}, {"Hilda", "153;"},
	{"Hektor", "624;"}, {"Achilles", "588;"},
	{"Chiron", "2060;"}, {"Chariklo", "10199;"}, {"Pholus", "5145;"}, {"Asbolus", "8405;"},
	{"Ixion", "28978;"}, {"Quaoar", "50000;"}, {"Haumea", "136108;"}, {"Makemake", "136472;"},
	{"Eris", "136199;"}, {"Sedna", "90377;"},
	{"1P/Halley", "DES=1P;CAP;NOFRAG"}, {"2P/Encke", "DES=2P;CAP;NOFRAG"},
	{"67P", "DES=67P;CAP;NOFRAG"}, {"29P", "DES=29P;CAP;NOFRAG"},
	{"C/1995 O1", "DES=C/1995 O1;CAP;NOFRAG"}, {"C/2014 UN271", "DES=C/2014 UN271;CAP;NOFRAG"},
	{"C/2023 A3", "DES=C/2023 A3;CAP;NOFRAG"},
}

// barycentricVectors queries Horizons for command's barycentric ICRF states at
// the TDB Julian dates jds, in AU and AU/day, in the order of jds.
func barycentricVectors(command string, jds []float64) (pos, vel []vector.Vec3, err error) {
	list := make([]string, len(jds))
	for i, jd := range jds {
		list[i] = fmt.Sprintf("'%.6f'", jd)
	}

	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%s'", command))
	params.Add("CENTER", "'@0'")
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'VECTORS'")
	params.Add("TLIST", strings.Join(list, " "))
	params.Add("TLIST_TYPE", "'JD'")
	params.Add("TIME_TYPE", "'TDB'")
	params.Add("OUT_UNITS", "'AU-D'")
	params.Add("REF_PLANE", "'FRAME'")
	params.Add("VEC_TABLE", "'2'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")

	block, err := horizonsGet(params)
	if err != nil {
		return nil, nil, err
	}

	byJD := make(map[float64][2]vector.Vec3)

	for line := range strings.SplitSeq(block, "\n") {
		cols := strings.Split(line, ",")
		if len(cols) < 8 {
			return nil, nil, fmt.Errorf("%w: vector output had %d in %q", errUnexpectedColumns, len(cols), line)
		}

		var v [7]float64

		for i, col := range []int{0, 2, 3, 4, 5, 6, 7} {
			if v[i], err = strconv.ParseFloat(strings.TrimSpace(cols[col]), 64); err != nil {
				return nil, nil, fmt.Errorf("parse vector row %q: %w", line, err)
			}
		}

		byJD[math.Round(v[0]*1e6)/1e6] = [2]vector.Vec3{vector.V3(v[1], v[2], v[3]), vector.V3(v[4], v[5], v[6])}
	}

	for _, jd := range jds {
		state, ok := byJD[math.Round(jd*1e6)/1e6]
		if !ok {
			return nil, nil, fmt.Errorf("%w: %q at JD %.6f", errNoVectorRows, command, jd)
		}

		pos, vel = append(pos, state[0]), append(vel, state[1])
	}

	return pos, vel, nil
}

// horizonsSun is a base that places the Sun about the barycenter at one
// instant, as Horizons does — the Sun a JPL kernel base gives Register — and
// nothing else.
type horizonsSun struct {
	at       time.Time
	pos, vel vector.Vec3
}

func (b horizonsSun) Locate(id core.ID, t time.Time) (core.State, error) {
	if id != core.Sun || !t.Equal(b.at) {
		return core.State{}, errNotHere
	}

	return core.State{Pos: b.pos, Vel: b.vel, Frame: core.FrameICRS, Center: core.SolarSystemBarycenter}, nil
}

func (horizonsSun) Close() error { return nil }

// interiorMass is the central mass of the barycentric fit for a body at
// distance r AU from the Sun at t: the Sun's and every planet system's nearer
// the Sun, placed by the provider's SOFA base, the source Register's choice
// reads.
func interiorMass(t *testing.T, sofa *kepler.Provider, at time.Time, r float64) float64 {
	t.Helper()

	gm := constants.Ephemeris
	mass := constants.IAU.SunGravitationalParameter.Value

	for _, p := range []struct {
		ids []core.ID // the chain to the Sun
		gm  float64
	}{
		{[]core.ID{core.MercuryBarycenter}, gm.MercurySystemGravitationalParameter.Value},
		{[]core.ID{core.VenusBarycenter}, gm.VenusSystemGravitationalParameter.Value},
		{[]core.ID{core.EarthMoonBarycenter, core.Earth}, gm.EarthMoonGravitationalParameter.Value},
		{[]core.ID{core.MarsBarycenter}, gm.MarsSystemGravitationalParameter.Value},
		{[]core.ID{core.JupiterBarycenter}, gm.JupiterSystemGravitationalParameter.Value},
		{[]core.ID{core.SaturnBarycenter}, gm.SaturnSystemGravitationalParameter.Value},
		{[]core.ID{core.UranusBarycenter}, gm.UranusSystemGravitationalParameter.Value},
		{[]core.ID{core.NeptuneBarycenter}, gm.NeptuneSystemGravitationalParameter.Value},
	} {
		var helio vector.Vec3

		for _, id := range p.ids {
			st, err := sofa.Locate(id, at)
			if err != nil {
				t.Fatalf("locate %v: %v", id, err)
			}

			helio = helio.Add(st.Pos)
		}

		if helio.Norm() < r {
			mass += p.gm
		}
	}

	return mass
}

// TestRegisterChoosesTheBetterCenterAgainstHorizons holds Register's choice of
// center (#803) to Horizons' integrations of the 26 bodies above. From each
// body's state at the epoch it builds both fits Register chooses between — about
// the Sun, and about the barycenter with the Sun and every planet inside the
// body's distance as the central mass — and requires that what Register stores
// over a base with Horizons' Sun lands nearer Horizons, 30 days and a year
// either side, than the fit it passed over.
//
// The closest call is Hektor, a Trojan with Jupiter just inside its distance:
// the barycentric fit Register keeps was the better by 16% at 30 days and 3%
// at a year when this was written.
//
// Longer spans are not held: there a close approach can decide instead. The
// study (#803) found Apophis's barycentric fit the better at five years, across
// its 2029 pass by the Earth, and at ten both fits miss 67P by over 80 degrees.
func TestRegisterChoosesTheBetterCenterAgainstHorizons(t *testing.T) {
	requireHorizons(t)

	const jd0 = 2460676.5 // 2025-01-01 TDB, the study's epoch

	epoch := time.FromJDParts(jd0, 0, time.TDB)
	spans := []float64{30, 365.25}

	jds := make([]float64, 1, 1+2*len(spans))
	jds[0] = jd0

	for _, span := range spans {
		jds = append(jds, jd0-span, jd0+span)
	}

	sun, sunVel, err := barycentricVectors("10", jds)
	testutil.SkipOnUpstreamFailure(t, err)
	testutil.AssertNoError(t, err)

	sunCenter := kepler.CentralBody{ID: core.Sun, GM: constants.IAU.SunGravitationalParameter.Value}
	sofa := kepler.New()

	var compared, outages int

	for _, body := range centerBodies {
		pos, vel, err := barycentricVectors(body.command, jds)
		if err != nil {
			// One body's request costs that body; only an identified outage
			// is skipped.
			if reason, ok := testutil.UpstreamFailure(err); ok {
				t.Logf("%s: Horizons did not answer (%s): %v", body.name, reason, err)

				outages++

				continue
			}

			t.Fatalf("%s: %v", body.name, err)
		}

		r := pos[0].Sub(sun[0])

		given, err := kepler.FromState(epoch, r, vel[0].Sub(sunVel[0]), sunCenter)
		if err != nil {
			t.Fatalf("%s: heliocentric fit: %v", body.name, err)
		}

		barycentric, err := kepler.FromState(epoch, pos[0], vel[0], kepler.CentralBody{
			ID: core.SolarSystemBarycenter, GM: interiorMass(t, sofa, epoch, r.Norm()),
		})
		if err != nil {
			t.Fatalf("%s: barycentric fit: %v", body.name, err)
		}

		const id core.ID = 1_000_000

		provider := kepler.New(kepler.WithBase(horizonsSun{at: epoch, pos: sun[0], vel: sunVel[0]}))
		if err := provider.Register(id, given); err != nil {
			t.Fatalf("%s: Register: %v", body.name, err)
		}

		st, err := provider.Locate(id, epoch)
		if err != nil {
			t.Fatal(err)
		}

		chosen := st.Center

		compared++

		for i, span := range spans {
			// The worst of either side, in km, of each fit and of what
			// Register stores.
			var helioKm, baryKm, storedKm float64

			for _, k := range []int{1 + 2*i, 2 + 2*i} {
				at := time.FromJDParts(jds[k], 0, time.TDB)

				h, _, err := given.StateAt(at)
				if err != nil {
					t.Fatal(err)
				}

				b, _, err := barycentric.StateAt(at)
				if err != nil {
					t.Fatal(err)
				}

				got, err := provider.Locate(id, at)
				if err != nil {
					t.Fatal(err)
				}

				stored := got.Pos
				if got.Center == core.Sun {
					stored = stored.Add(sun[k])
				}

				helioKm = math.Max(helioKm, unit.AU(h.Add(sun[k]).Sub(pos[k]).Norm()).Km())
				baryKm = math.Max(baryKm, unit.AU(b.Sub(pos[k]).Norm()).Km())
				storedKm = math.Max(storedKm, unit.AU(stored.Sub(pos[k]).Norm()).Km())
			}

			passedOver := baryKm
			if chosen == core.SolarSystemBarycenter {
				passedOver = helioKm
			}

			t.Logf("%-12s r %5.2f AU, %6.2f d: about the Sun %9.3g km, the barycenter %9.3g km; stored about %v, %9.3g km",
				body.name, r.Norm(), span, helioKm, baryKm, chosen, storedKm)

			if !(storedKm < passedOver) {
				t.Errorf("%s at %.2f days: Register fitted about %v, %.3g km from Horizons, where the fit it passed over is %.3g km",
					body.name, span, chosen, storedKm, passedOver)
			}
		}
	}

	t.Logf("%d bodies compared, %d unanswered", compared, outages)

	if compared == 0 {
		t.Skip("Horizons answered for none of the bodies")
	}
}
