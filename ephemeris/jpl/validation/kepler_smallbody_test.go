//go:build network

package jpl_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/TuSKan/astrogo/catalog/sbdb"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// keplerSmallBodyReference records what the two-body propagation is measured
// against.
//
// SBDB publishes the elements and Horizons generates the kernel from the same
// orbit solution, so this is not an independent check of where the asteroid
// is. It measures something more useful to a caller: how much is lost by
// propagating the published elements as unperturbed two-body motion instead
// of reading the integrated kernel — the exact choice plan.NewAsteroid makes
// when no kernel is available.
func keplerSmallBodyReference() metrology.Reference {
	return metrology.Reference{
		Kind:           metrology.KindSPICE,
		Name:           "JPL Horizons SPK (Type 21)",
		Version:        "Horizons-generated small-body kernel",
		Source:         "https://ssd.jpl.nasa.gov/api/horizons.api",
		Dataset:        "two-body propagation of SBDB osculating elements against the kernel",
		SharedAncestor: "JPL small-body solution — SBDB and the kernel are one orbit fit",
	}
}

// keplerSmallBodySpanDays is how far either side of the epoch of osculation
// the comparison reaches.
//
// Osculating elements are exact at their own epoch by construction, so the
// error at dt = 0 measures only the published elements' rounding, and
// everything beyond it measures accumulated planetary perturbation. Thirty
// days is the span the package's own Eros test uses and is long enough for
// the perturbation to dominate the rounding by an order of magnitude.
const keplerSmallBodySpanDays = 30

// keplerSmallBodyContract bounds what two-body propagation of published
// osculating elements is worth over a month.
//
// # Why there is no published figure
//
// Nobody publishes "the error of propagating our elements two-body", because
// the elements are not published for that purpose — they are a statement of
// the orbit at one instant, and the kernel is what JPL offers for positions.
// So this takes the construction the Galilean and Pluto suites use: a bound
// between two measured errors, chosen so it fails for a structural fault and
// passes while the perturbations are merely unmodelled.
//
//   - Below it, the divergence this package openly does not model: 1.777e-5 AU
//     (2,658 km) at worst over the six bodies and the window sampled here. The
//     shape is the confirmation that this is what is being measured — the
//     error is smallest at dt = 0, where osculating elements are exact by
//     construction, and grows as t² either side of it.
//   - Above it, a structural fault. Reading the elements one frame out — the
//     realistic mistake, since they are published against the ecliptic and the
//     result is returned in equatorial ICRS — displaces these bodies by at
//     least 0.130 AU. See TestSmallBodyElementsCarryTheObliquityRotation.
//
// The bound is the geometric mean of the two, so it is pinned to neither.
//
// # What this suite found on its first run
//
// That dt = 0 minimum is not decoration; it is the assertion that caught a
// real bug. The first run measured 690,000 km at the epoch of osculation for
// Eros and 2.5 million km for Phaethon, with no minimum at dt = 0 at all —
// which cannot be perturbation, because there is nothing yet to perturb.
//
// The cause was in catalog/sbdb: SBDB rounds every orbital element to three
// significant figures unless the request asks otherwise, so Eros resolved with
// a = 1.46 rather than 1.458243716. Every plausible-band assertion in that
// package still passed. Requesting full precision moved the dt = 0 residual
// from 690,000 km to 21 km, a factor of about 1,800, and it is guarded now by
// TestSBDBElementsAreFullPrecision.
//
// The contract below is written against the fixed behaviour. Had this suite
// been written to accept what was measured first, it would have recorded a
// 0.0123 AU bound and called a parsing bug the two-body approximation.
func keplerSmallBodyContract() metrology.Contract {
	return metrology.MustContract(keplerSmallBodyBoundAU, "AU",
		"not an accuracy claim. Two-body propagation of osculating elements is wrong here by "+
			"construction: the elements are exact only at their epoch of osculation and the "+
			"planetary perturbations that move them are unmodelled, which costs 1.777e-5 AU "+
			"(2,658 km) at worst over the 30 days either side sampled. Reading the same elements "+
			"one frame out instead — the realistic structural mistake, since they are published "+
			"against the ecliptic and returned in equatorial ICRS — displaces these bodies by at "+
			"least 0.130 AU. This bound is the geometric mean, 84x above the first and 87x below "+
			"the second, so it fails when the elements or the frame are wrong and passes while "+
			"the perturbations are merely absent",
		"both anchors measured: this suite for the divergence, "+
			"TestSmallBodyElementsCarryTheObliquityRotation for the structural one")
}

// keplerSmallBodyBoundAU is the geometric mean of the two anchors above,
// sqrt(1.777e-5 * 0.130), to two figures.
const keplerSmallBodyBoundAU = 1.5e-3

// TestKeplerSmallBodyAgainstSPK measures what plan.NewAsteroid is worth
// without a kernel.
//
// # Why this is the suite a caller most needs
//
// A small body is the case where astrogo has two genuinely different answers
// and the choice is the caller's: six numbers from SBDB propagated two-body,
// or a Horizons-generated kernel read directly. The kernel path already has a
// row — ephemeris.jpl.smallbody.position, at 2e-13 AU — but that measures
// astrogo's Type 21 decoder against Horizons' own vectors, not the orbit. The
// elements path had no row at all, so the cost of choosing it was unstated.
//
// The two rows answer different questions and belong side by side: one says
// the decoder is right, this one says what the approximation costs.
func TestKeplerSmallBodyAgainstSPK(t *testing.T) {
	suite := metrology.NewSuite("ephemeris.kepler.smallbody",
		keplerSmallBodyReference(), keplerSmallBodyContract())

	if !testutil.Reachable(horizonsHost) {
		metrology.NotVerified(t, "JPL Horizons is unreachable", suite)
	}

	provider := sbdb.New()

	for _, body := range smallBodies() {
		t.Run(body.name, func(t *testing.T) {
			el, epoch, ok := smallBodyElements(t, provider, body)
			if !ok {
				return
			}

			// The kernel has to cover the whole comparison plus a margin:
			// asking for a state on a segment boundary fails outright, which
			// is how the sibling suite first met "no coverage for target at
			// requested epoch".
			const marginDays = 5

			start := epoch.AddDays(-keplerSmallBodySpanDays - marginDays)
			stop := epoch.AddDays(keplerSmallBodySpanDays + marginDays)

			p, err := jpl.NewProvider(context.Background(), core.SmallBody, body.designation,
				jpl.WithTimeInterval(start, stop))
			if err != nil {
				if errors.Is(err, spk.ErrHorizonsEmptyKernel) {
					t.Skipf("Horizons returned an unusable SPK for %s: %v", body.name, err)
				}

				t.Skipf("provider for %s: %v", body.name, err)
			}

			defer func() { _ = p.Close() }()

			for dt := -keplerSmallBodySpanDays; dt <= keplerSmallBodySpanDays; dt += 5 {
				at := epoch.AddDays(float64(dt))

				want, werr := p.State(core.SmallBodyID(body.id), at)
				if werr != nil {
					t.Errorf("%s: kernel State at dt=%+d: %v", body.name, dt, werr)

					continue
				}

				got, gerr := keplerProvider(t, body.id, el).State(core.SmallBodyID(body.id), at)
				if gerr != nil {
					t.Errorf("%s: kepler State at dt=%+d: %v", body.name, dt, gerr)

					continue
				}

				diff := got.Pos.Sub(want.Pos).Norm()

				suite.Add(metrology.Sample{
					Error: diff,
					Label: body.name,
					Context: fmt.Sprintf("dt=%+dd from the epoch of osculation, "+
						"|kernel - kepler| = %.0f km", dt, diff*kmPerAU),
				})
			}

			t.Logf("%s (%s): %s", body.name, body.designation, body.why)
		})
	}

	if suite.Len() == 0 {
		metrology.NotVerified(t, "no small body produced a comparable state", suite)
	}

	suite.Report(t)
}

// smallBodyElements resolves one body's published osculating elements.
func smallBodyElements(t *testing.T, provider *sbdb.Provider, body smallBody) (
	kepler.Elements, time.Time, bool,
) {
	t.Helper()

	target, err := provider.Resolve(context.Background(), strconv.Itoa(body.id))
	if err != nil {
		t.Skipf("SBDB did not resolve %d (%s)", body.id, body.name)

		return kepler.Elements{}, time.Time{}, false
	}

	if !target.HasElements {
		t.Skipf("SBDB returned no elements for %s", body.name)

		return kepler.Elements{}, time.Time{}, false
	}

	el, err := kepler.NewElements(target.Epoch, target.SemiMajorAxis, target.Eccentricity,
		target.Inclination, target.AscendingNode, target.ArgPeriapsis, target.MeanAnomaly)
	if err != nil {
		t.Errorf("%s: NewElements from SBDB: %v", body.name, err)

		return kepler.Elements{}, time.Time{}, false
	}

	return el, target.Epoch, true
}

// keplerProvider wraps one body's elements in a provider, so both sides of
// the comparison are reached through the same core.Provider interface and the
// heliocentric-to-geocentric conversion is the library's rather than the
// test's.
func keplerProvider(t *testing.T, id int, el kepler.Elements) *kepler.Provider {
	t.Helper()

	p := kepler.New()
	if err := p.Register(core.SmallBodyID(id), el); err != nil {
		t.Fatalf("Register: %v", err)
	}

	return p
}

// TestSmallBodyElementsCarryTheObliquityRotation measures the upper anchor
// [keplerSmallBodyContract] rests on.
//
// Same fault as the Pluto case and the same reason it is worth a test: the
// elements are published against the J2000 ecliptic and StateAt returns
// equatorial ICRS, so exactly one obliquity rotation stands between them, and
// omitting it leaves an orbit of the right size and shape in the wrong place.
func TestSmallBodyElementsCarryTheObliquityRotation(t *testing.T) {
	const obliquityDeg = 23.4392911

	obliquity := obliquityDeg * math.Pi / 180
	sinE, cosE := math.Sin(obliquity), math.Cos(obliquity)

	provider := sbdb.New()

	if !testutil.Reachable(horizonsHost) {
		t.Skip("JPL Horizons is unreachable")
	}

	smallest := math.Inf(1)

	for _, body := range smallBodies() {
		el, epoch, ok := smallBodyElements(t, provider, body)
		if !ok {
			continue
		}

		for dt := -keplerSmallBodySpanDays; dt <= keplerSmallBodySpanDays; dt += 5 {
			pos, _, err := el.StateAt(epoch.AddDays(float64(dt)))
			if err != nil {
				t.Errorf("%s: StateAt: %v", body.name, err)

				continue
			}

			unrotated := vector.V3(pos.X, pos.Y*cosE+pos.Z*sinE, -pos.Y*sinE+pos.Z*cosE)
			smallest = math.Min(smallest, pos.Sub(unrotated).Norm())
		}
	}

	if math.IsInf(smallest, 1) {
		t.Skip("no body resolved")
	}

	t.Logf("a missing obliquity rotation displaces these bodies by at least %.3f AU", smallest)

	// A wide range, not an equality: the displacement depends on where each
	// body happens to be relative to the rotation axis, and the point is only
	// that it stays four orders of magnitude above the divergence the contract
	// is meant to tolerate.
	if smallest < 0.05 || smallest > 0.5 {
		t.Errorf("smallest structural displacement is %.3f AU, expected about 0.130 — "+
			"keplerSmallBodyContract's upper anchor no longer holds", smallest)
	}
}
