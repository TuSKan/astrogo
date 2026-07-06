//go:build integration

// Package magnitude_test contains integration tests that validate astrogo's
// astronomical computations against the IMCCE SSOCard API.
//
// Run with: go test -tags integration -run TestIMCCE -v ./magnitude/
//
// These tests require an active internet connection to reach
// https://ssp.imcce.fr/webservices/ssodnet/api/ssocard/ endpoints.
package magnitude_test

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/magnitude"
)

// ══════════════════════════════════════════════════════════════════════════════
// IMCCE SSOCard Validation
// These tests fetch reference H,G values from ssp.imcce.fr and validate
// our HG model against them. They require network access.
// ══════════════════════════════════════════════════════════════════════════════

// imcceCard mirrors the actual SSOCard schema (see
// https://ssp.imcce.fr/webservices/ssodnet/api/ssocard/<name>): H and G are
// each their own nested {value, error} object *inside* absolute_magnitude,
// not directly on it — e.g. parameters.physical.absolute_magnitude.H.value,
// not parameters.physical.absolute_magnitude.value. There is no separate
// top-level "phase_slope" field; G lives alongside H under the same object.
type imcceCard struct {
	Title  string `json:"title"`
	Params struct {
		Physical struct {
			AbsMag *struct {
				H struct {
					Value float64 `json:"value"`
				} `json:"H"`
				G struct {
					Value float64 `json:"value"`
				} `json:"G"`
			} `json:"absolute_magnitude"`
		} `json:"physical"`
	} `json:"parameters"`
}

func fetchIMCCE(t *testing.T, name string) (H, G float64) {
	t.Helper()
	resp, err := http.Get("https://ssp.imcce.fr/webservices/ssodnet/api/ssocard/" + name)
	if err != nil {
		t.Skipf("IMCCE network unavailable: %v", err)
	}
	defer resp.Body.Close()

	var card imcceCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Skipf("IMCCE JSON decode error: %v", err)
	}

	if card.Params.Physical.AbsMag == nil {
		t.Skipf("IMCCE: no H value for %s", name)
	}

	H = card.Params.Physical.AbsMag.H.Value
	G = card.Params.Physical.AbsMag.G.Value

	// No known minor planet has H <= 0 (that would be brighter than Venus
	// at 1 AU). This endpoint is reachable and its JSON decodes cleanly by
	// this point, so per this project's network-test convention, an
	// implausible value here is our own logic error against valid data —
	// fail loudly, don't skip it.
	if H <= 0 {
		t.Fatalf("IMCCE: implausible H=%.3f for %s from a reachable, decodable response — check imcceCard's field mapping against the live schema", H, name)
	}

	t.Logf("IMCCE %s: H=%.3f G=%.3f", card.Title, H, G)
	return H, G
}

func TestIMCCE_CeresOpposition(t *testing.T) {
	H, G := fetchIMCCE(t, "Ceres")

	// Ceres at opposition: r≈2.77 AU, Δ≈1.77 AU, α≈0°.
	r, delta := 2.77, 1.77
	mag := magnitude.AsteroidHG(H, G, r, delta, angle.Deg(0))
	expected := H + 5*math.Log10(r*delta)

	if math.Abs(mag-expected) > 0.01 {
		t.Errorf("Ceres at opposition: got %.3f, expected %.3f", mag, expected)
	}
	t.Logf("Ceres opposition: V=%.2f (H=%.3f from IMCCE)", mag, H)
}

func TestIMCCE_VestaOpposition(t *testing.T) {
	H, G := fetchIMCCE(t, "Vesta")

	// Vesta at opposition: r≈2.36 AU, Δ≈1.36 AU, α≈0°.
	r, delta := 2.36, 1.36
	mag := magnitude.AsteroidHG(H, G, r, delta, angle.Deg(0))
	expected := H + 5*math.Log10(r*delta)

	if math.Abs(mag-expected) > 0.01 {
		t.Errorf("Vesta at opposition: got %.3f, expected %.3f", mag, expected)
	}
	// Vesta can reach V≈5.1 at closest opposition — verify physically.
	if mag > 8 || mag < 3 {
		t.Errorf("Vesta V=%.2f out of physical range", mag)
	}
	t.Logf("Vesta opposition: V=%.2f (H=%.3f from IMCCE)", mag, H)
}

func TestIMCCE_ErosPhaseEffect(t *testing.T) {
	H, G := fetchIMCCE(t, "Eros")

	// Eros at close approach: r≈1.5 AU, Δ≈0.5 AU.
	r, delta := 1.5, 0.5
	m0 := magnitude.AsteroidHG(H, G, r, delta, angle.Deg(0))
	m30 := magnitude.AsteroidHG(H, G, r, delta, angle.Deg(30))
	m60 := magnitude.AsteroidHG(H, G, r, delta, angle.Deg(60))

	// Phase effect should be monotonic.
	if m30 <= m0 || m60 <= m30 {
		t.Errorf("Eros phase not monotonic: α=0°:%.2f, 30°:%.2f, 60°:%.2f", m0, m30, m60)
	}

	// Phase correction at 30° should be ~0.5-1.5 mag for typical G.
	phaseCorrection := m30 - m0
	if phaseCorrection < 0.3 || phaseCorrection > 3.0 {
		t.Errorf("Eros phase correction at 30°: %.2f mag (expected 0.3-3.0)", phaseCorrection)
	}

	t.Logf("Eros (IMCCE H=%.2f G=%.2f): α=0°:%.2f, 30°:%.2f (Δm=%.2f), 60°:%.2f",
		H, G, m0, m30, phaseCorrection, m60)
}

func TestIMCCE_MultiBodyConsistency(t *testing.T) {
	// Verify that all five asteroids produce reasonable magnitudes at a common geometry.
	asteroids := []string{"Ceres", "Vesta", "Pallas", "Juno", "Eros"}
	r, delta := 3.0, 2.0

	for _, name := range asteroids {
		H, G := fetchIMCCE(t, name)
		mag := magnitude.AsteroidHG(H, G, r, delta, angle.Deg(10))

		// All should be in reasonable range (0 to 20 mag at these distances).
		if mag < 0 || mag > 25 {
			t.Errorf("%s: V=%.2f out of range (H=%.3f G=%.3f)", name, mag, H, G)
		}
		t.Logf("%s: V=%.2f at r=%.1f Δ=%.1f α=10° (H=%.3f G=%.3f)", name, mag, r, delta, H, G)
	}
}
