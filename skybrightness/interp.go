package skybrightness

import "sort"

// grid2D is a rectilinear (possibly non-uniformly spaced) table of values with
// bilinear interpolation. xs and ys are the strictly increasing breakpoints of
// the two axes; v[i][j] is the value at (xs[i], ys[j]). Queries outside the
// covered range are clamped to the nearest edge.
type grid2D struct {
	xs []float64
	ys []float64
	v  [][]float64
}

// at returns the bilinearly interpolated value at (x, y), clamping to the grid
// edges. At a breakpoint it reproduces the stored cell value exactly.
func (g grid2D) at(x, y float64) float64 {
	i, fx := locate(g.xs, x)
	j, fy := locate(g.ys, y)

	v00 := g.v[i][j]
	v10 := g.v[i+1][j]
	v01 := g.v[i][j+1]
	v11 := g.v[i+1][j+1]

	a := v00 + fx*(v10-v00)
	b := v01 + fx*(v11-v01)

	return a + fy*(b-a)
}

// locate returns the lower cell index i (such that xs[i] <= x <= xs[i+1] after
// clamping) and the fractional position fx in [0,1] within that cell.
func locate(xs []float64, x float64) (int, float64) {
	n := len(xs)

	if x <= xs[0] {
		return 0, 0
	}

	if x >= xs[n-1] {
		return n - 2, 1
	}

	// xs[i] <= x < xs[i+1]
	i := sort.SearchFloat64s(xs, x)
	if xs[i] == x {
		// Exact breakpoint hit; use the cell starting at i (clamp the top edge).
		if i == n-1 {
			return n - 2, 1
		}

		return i, 0
	}

	i--

	return i, (x - xs[i]) / (xs[i+1] - xs[i])
}
