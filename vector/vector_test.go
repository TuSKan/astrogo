package vector_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/vector"
)

const tol = 1e-13

// ── V3 / Zero ─────────────────────────────────────────────────────────────────

func TestV3(t *testing.T) {
	v := vector.V3(1, 2, 3)
	if v.X != 1 || v.Y != 2 || v.Z != 3 {
		t.Errorf("V3(1,2,3) = %+v, want {1 2 3}", v)
	}
}

func TestZero(t *testing.T) {
	z := vector.Zero()
	if z.X != 0 || z.Y != 0 || z.Z != 0 {
		t.Errorf("Zero() = %+v, want {0 0 0}", z)
	}
}

// ── Arithmetic ────────────────────────────────────────────────────────────────

func TestAdd(t *testing.T) {
	a := vector.V3(1, 2, 3)
	b := vector.V3(4, 5, 6)
	got := a.Add(b)
	want := vector.V3(5, 7, 9)
	assertVecNear(t, "Add", got, want, tol)
}

func TestSub(t *testing.T) {
	a := vector.V3(4, 5, 6)
	b := vector.V3(1, 2, 3)
	assertVecNear(t, "Sub", a.Sub(b), vector.V3(3, 3, 3), tol)
}

func TestMulScalar(t *testing.T) {
	v := vector.V3(1, -2, 3)
	assertVecNear(t, "MulScalar(2)", v.MulScalar(2), vector.V3(2, -4, 6), tol)
	assertVecNear(t, "MulScalar(0)", v.MulScalar(0), vector.Zero(), tol)
	assertVecNear(t, "MulScalar(-1)", v.MulScalar(-1), vector.V3(-1, 2, -3), tol)
}

func TestDivScalar(t *testing.T) {
	v := vector.V3(2, -4, 6)
	assertVecNear(t, "DivScalar(2)", v.DivScalar(2), vector.V3(1, -2, 3), tol)
}

// ── Dot and Cross ─────────────────────────────────────────────────────────────

func TestDot(t *testing.T) {
	cases := []struct {
		name string
		a, b vector.Vec3
		want float64
	}{
		{"parallel", vector.V3(1, 0, 0), vector.V3(1, 0, 0), 1},
		{"anti-parallel", vector.V3(1, 0, 0), vector.V3(-1, 0, 0), -1},
		{"orthogonal XY", vector.V3(1, 0, 0), vector.V3(0, 1, 0), 0},
		{"general", vector.V3(1, 2, 3), vector.V3(4, 5, 6), 32},
		{"zero", vector.Zero(), vector.V3(1, 2, 3), 0},
	}
	for i, c := range cases {
		testutil.AssertNear(t, testutil.CaseLabel(i, c.name), c.a.Dot(c.b), c.want, tol)
	}
}

func TestCross_basis(t *testing.T) {
	x := vector.V3(1, 0, 0)
	y := vector.V3(0, 1, 0)
	z := vector.V3(0, 0, 1)

	assertVecNear(t, "X×Y=Z", x.Cross(y), z, tol)
	assertVecNear(t, "Y×Z=X", y.Cross(z), x, tol)
	assertVecNear(t, "Z×X=Y", z.Cross(x), y, tol)

	// Anti-commutativity: v×w = -(w×v)
	assertVecNear(t, "Y×X=-Z", y.Cross(x), z.MulScalar(-1), tol)
}

func TestCross_orthogonal(t *testing.T) {
	// Cross product must be orthogonal to both inputs.
	a := vector.V3(1, 2, 3)
	b := vector.V3(4, -5, 6)
	c := a.Cross(b)
	testutil.AssertNear(t, "c·a=0", c.Dot(a), 0, tol)
	testutil.AssertNear(t, "c·b=0", c.Dot(b), 0, tol)
}

func TestCross_self_is_zero(t *testing.T) {
	v := vector.V3(3, -1, 2)
	assertVecNear(t, "v×v=0", v.Cross(v), vector.Zero(), tol)
}

// ── Norm ──────────────────────────────────────────────────────────────────────

func TestNorm2(t *testing.T) {
	testutil.AssertNear(t, "Norm2(1,2,2)", vector.V3(1, 2, 2).Norm2(), 9, tol)
	testutil.AssertNear(t, "Norm2(zero)", vector.Zero().Norm2(), 0, tol)
}

func TestNorm(t *testing.T) {
	testutil.AssertNear(t, "Norm(3,4,0)=5", vector.V3(3, 4, 0).Norm(), 5, tol)
	testutil.AssertNear(t, "Norm(zero)=0", vector.Zero().Norm(), 0, tol)
	testutil.AssertNear(t, "Norm(1,1,1)=√3", vector.V3(1, 1, 1).Norm(), math.Sqrt(3), tol)
}

func TestUnit(t *testing.T) {
	// Non-zero vector: result must have unit norm and same direction.
	v := vector.V3(3, 4, 0)
	u := v.Unit()
	testutil.AssertNear(t, "unit norm=1", u.Norm(), 1.0, tol)
	// Direction: Unit(v) must be parallel to v, so their cross product is zero.
	assertVecNear(t, "unit direction", u.Cross(v), vector.Zero(), tol)

	// Zero vector: Unit must return Zero() without panicking.
	assertVecNear(t, "Unit(zero)=zero", vector.Zero().Unit(), vector.Zero(), tol)
}

// ── Spherical coordinates ─────────────────────────────────────────────────────

func TestFromSpherical_unitNorm(t *testing.T) {
	lons := []float64{0, math.Pi / 4, math.Pi / 2, math.Pi, 3 * math.Pi / 2}
	lats := []float64{0, math.Pi / 6, math.Pi / 3, -math.Pi / 4, -math.Pi / 3}
	for _, lon := range lons {
		for _, lat := range lats {
			n := vector.FromSpherical(lon, lat).Norm()
			testutil.AssertNear(t, "FromSpherical norm=1", n, 1.0, tol)
		}
	}
}

func TestFromSpherical_knownPoints(t *testing.T) {
	cases := []struct {
		name     string
		lon, lat float64
		want     vector.Vec3
	}{
		{"+X axis", 0, 0, vector.V3(1, 0, 0)},
		{"+Y axis", math.Pi / 2, 0, vector.V3(0, 1, 0)},
		{"-X axis", math.Pi, 0, vector.V3(-1, 0, 0)},
		{"-Y axis", 3 * math.Pi / 2, 0, vector.V3(0, -1, 0)},
		{"North pole", 0, math.Pi / 2, vector.V3(0, 0, 1)},
		{"South pole", 0, -math.Pi / 2, vector.V3(0, 0, -1)},
	}
	for i, c := range cases {
		assertVecNear(t, testutil.CaseLabel(i, c.name), vector.FromSpherical(c.lon, c.lat), c.want, tol)
	}
}

func TestToSpherical_roundTrip(t *testing.T) {
	cases := []struct {
		lon, lat float64
	}{
		{0, 0},
		{math.Pi / 4, math.Pi / 6},
		{math.Pi / 2, math.Pi / 3},
		{math.Pi, -math.Pi / 4},
		{3 * math.Pi / 2, -math.Pi / 6},
		{2*math.Pi - 0.01, 0.5},
	}
	for i, c := range cases {
		v := vector.FromSpherical(c.lon, c.lat)
		gotLon, gotLat := v.ToSpherical()
		testutil.AssertAngleNear(t, testutil.CaseLabel(i, "lon"), gotLon, c.lon, tol)
		testutil.AssertNear(t, testutil.CaseLabel(i, "lat"), gotLat, c.lat, tol)
	}
}

func TestToSpherical_poles(t *testing.T) {
	// At the poles, longitude is undefined; we just verify lat is correct.
	_, lat := vector.FromSpherical(1.23, math.Pi/2).ToSpherical()
	testutil.AssertNear(t, "north pole lat", lat, math.Pi/2, tol)

	_, lat = vector.FromSpherical(2.34, -math.Pi/2).ToSpherical()
	testutil.AssertNear(t, "south pole lat", lat, -math.Pi/2, tol)
}

func TestToSpherical_zeroVector(t *testing.T) {
	lon, lat := vector.Zero().ToSpherical()
	testutil.AssertExact(t, "zero lon", lon, 0)
	testutil.AssertExact(t, "zero lat", lat, 0)
}

// ── Rotations ─────────────────────────────────────────────────────────────────

func TestRotateZ_quarterTurn(t *testing.T) {
	// Rotating X-axis by π/2 about Z gives Y-axis.
	x := vector.V3(1, 0, 0)
	assertVecNear(t, "RotateZ(π/2)(X)=Y", x.RotateZ(math.Pi/2), vector.V3(0, 1, 0), tol)
}

func TestRotateZ_halfTurn(t *testing.T) {
	// Rotating X-axis by π about Z gives -X.
	x := vector.V3(1, 0, 0)
	assertVecNear(t, "RotateZ(π)(X)=-X", x.RotateZ(math.Pi), vector.V3(-1, 0, 0), tol)
}

func TestRotateX_halfTurn(t *testing.T) {
	// Rotating Y-axis by π about X gives -Y.
	y := vector.V3(0, 1, 0)
	assertVecNear(t, "RotateX(π)(Y)=-Y", y.RotateX(math.Pi), vector.V3(0, -1, 0), tol)
}

func TestRotateY_quarterTurn(t *testing.T) {
	// Rotating Z-axis by π/2 about Y gives X-axis.
	z := vector.V3(0, 0, 1)
	assertVecNear(t, "RotateY(π/2)(Z)=X", z.RotateY(math.Pi/2), vector.V3(1, 0, 0), tol)
}

func TestRotate_zeroAngle_identity(t *testing.T) {
	v := vector.V3(1, 2, 3)
	assertVecNear(t, "RotateX(0)=v", v.RotateX(0), v, tol)
	assertVecNear(t, "RotateY(0)=v", v.RotateY(0), v, tol)
	assertVecNear(t, "RotateZ(0)=v", v.RotateZ(0), v, tol)
}

func TestRotate_preservesNorm(t *testing.T) {
	// Rotations must preserve vector length.
	v := vector.V3(1, 2, 3)
	n := v.Norm()
	testutil.AssertNear(t, "|RotX(v)|=|v|", v.RotateX(1.23).Norm(), n, tol)
	testutil.AssertNear(t, "|RotY(v)|=|v|", v.RotateY(2.34).Norm(), n, tol)
	testutil.AssertNear(t, "|RotZ(v)|=|v|", v.RotateZ(3.45).Norm(), n, tol)
}

func TestRotateZ_composition(t *testing.T) {
	// Two quarter-turns about Z == one half-turn.
	v := vector.V3(1, 0.5, -0.5)
	twoQuarters := v.RotateZ(math.Pi / 2).RotateZ(math.Pi / 2)
	half := v.RotateZ(math.Pi)
	assertVecNear(t, "2×π/2 == π", twoQuarters, half, tol)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// assertVecNear fails t if any component of got differs from want by more than tol.
func assertVecNear(t testing.TB, label string, got, want vector.Vec3, tolerance float64) {
	t.Helper()
	testutil.AssertNear(t, label+".X", got.X, want.X, tolerance)
	testutil.AssertNear(t, label+".Y", got.Y, want.Y, tolerance)
	testutil.AssertNear(t, label+".Z", got.Z, want.Z, tolerance)
}
