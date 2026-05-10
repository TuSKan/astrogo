package vector

import "math"

// Vec3 is a 3-dimensional Cartesian vector.
//
// It is a value type: copy and compare freely. All methods return new Vec3
// values; nothing is mutated in place.
type Vec3 struct {
	X, Y, Z float64
}

// ── Constructors ──────────────────────────────────────────────────────────────

// V3 constructs a Vec3 from its three components.
func V3(x, y, z float64) Vec3 { return Vec3{x, y, z} }

// Zero returns the zero vector (0, 0, 0).
func Zero() Vec3 { return Vec3{} }

// ── Arithmetic ────────────────────────────────────────────────────────────────

// Add returns v + w.
func (v Vec3) Add(w Vec3) Vec3 {
	return Vec3{v.X + w.X, v.Y + w.Y, v.Z + w.Z}
}

// Sub returns v - w.
func (v Vec3) Sub(w Vec3) Vec3 {
	return Vec3{v.X - w.X, v.Y - w.Y, v.Z - w.Z}
}

// MulScalar returns v × s.
func (v Vec3) MulScalar(s float64) Vec3 {
	return Vec3{v.X * s, v.Y * s, v.Z * s}
}

// DivScalar returns v / s.
// Returns a NaN vector if s == 0 (consistent with float64 arithmetic).
func (v Vec3) DivScalar(s float64) Vec3 {
	return Vec3{v.X / s, v.Y / s, v.Z / s}
}

// ── Products ──────────────────────────────────────────────────────────────────

// Dot returns the scalar dot product v · w.
func (v Vec3) Dot(w Vec3) float64 {
	return v.X*w.X + v.Y*w.Y + v.Z*w.Z
}

// Cross returns the vector cross product v × w.
// The result is orthogonal to both v and w.
func (v Vec3) Cross(w Vec3) Vec3 {
	return Vec3{
		X: v.Y*w.Z - v.Z*w.Y,
		Y: v.Z*w.X - v.X*w.Z,
		Z: v.X*w.Y - v.Y*w.X,
	}
}

// ── Norm ──────────────────────────────────────────────────────────────────────

// Norm2 returns |v|², the squared Euclidean norm.
// Cheaper than Norm when only comparison is needed.
func (v Vec3) Norm2() float64 {
	return v.X*v.X + v.Y*v.Y + v.Z*v.Z
}

// Norm returns |v|, the Euclidean norm.
func (v Vec3) Norm() float64 {
	return math.Sqrt(v.Norm2())
}

// Unit returns a unit vector in the direction of v.
// If v is the zero vector, Unit returns Zero() without panicking.
func (v Vec3) Unit() Vec3 {
	n := v.Norm()
	if n == 0 {
		return Zero()
	}

	return v.DivScalar(n)
}

// ── Spherical coordinates ─────────────────────────────────────────────────────

// FromSpherical constructs a unit Vec3 from spherical coordinates.
//   - lon is the longitude (azimuth, right ascension) in radians.
//   - lat is the latitude (elevation, declination) in radians, in [-π/2, π/2].
//
// The resulting vector has unit norm for finite, non-NaN inputs.
func FromSpherical(lon, lat float64) Vec3 {
	cosLat := math.Cos(lat)

	return Vec3{
		X: cosLat * math.Cos(lon),
		Y: cosLat * math.Sin(lon),
		Z: math.Sin(lat),
	}
}

// ToSpherical converts v to spherical coordinates (lon, lat) in radians.
//
//   - lon ∈ [0, 2π) is the longitude (azimuth).
//   - lat ∈ [-π/2, π/2] is the latitude (elevation).
//
// For the zero vector, (0, 0) is returned.
// At the poles (X == Y == 0), lon is defined as 0.
func (v Vec3) ToSpherical() (lon, lat float64) {
	n := v.Norm()
	if n == 0 {
		return 0, 0
	}

	lat = math.Asin(v.Z / n)

	lon = math.Atan2(v.Y, v.X)
	if lon < 0 {
		lon += 2 * math.Pi
	}

	return lon, lat
}

// ── Rotations ─────────────────────────────────────────────────────────────────
// Each Rotate* method applies a right-hand (counter-clockwise) rotation about
// the named axis viewed from the positive end of that axis.

// RotateX returns v rotated by rad radians about the X axis.
//
//	X' = X
//	Y' = cos·Y − sin·Z
//	Z' = sin·Y + cos·Z
func (v Vec3) RotateX(rad float64) Vec3 {
	c, s := math.Cos(rad), math.Sin(rad)

	return Vec3{
		X: v.X,
		Y: c*v.Y - s*v.Z,
		Z: s*v.Y + c*v.Z,
	}
}

// RotateY returns v rotated by rad radians about the Y axis.
//
//	X' = cos·X + sin·Z
//	Y' = Y
//	Z' = −sin·X + cos·Z
func (v Vec3) RotateY(rad float64) Vec3 {
	c, s := math.Cos(rad), math.Sin(rad)

	return Vec3{
		X: c*v.X + s*v.Z,
		Y: v.Y,
		Z: -s*v.X + c*v.Z,
	}
}

// RotateZ returns v rotated by rad radians about the Z axis.
//
//	X' = cos·X − sin·Y
//	Y' = sin·X + cos·Y
//	Z' = Z
func (v Vec3) RotateZ(rad float64) Vec3 {
	c, s := math.Cos(rad), math.Sin(rad)

	return Vec3{
		X: c*v.X - s*v.Y,
		Y: s*v.X + c*v.Y,
		Z: v.Z,
	}
}
