package atlas

import (
	"errors"
	"io"

	"github.com/TuSKan/astrogo/skybrightness"
)

// ErrLorenzNoNumericData reports that the Lorenz Light Pollution Atlas is not
// available as a clean numeric grid.
var ErrLorenzNoNumericData = errors.New(
	"atlas: Lorenz LPA numeric grid not published (PNG zone maps only); " +
		"use NewFalchiProvider (WA sky brightness) or the VIIRS providers (raw radiance)")

// NewLorenzProvider is a placeholder for the David Lorenz "Light Pollution
// Atlas" (djlorenz.github.io/astronomy/lp/) — the freshest propagated atlas
// (annual through 2024/2025), in the same mcd/m² artificial-brightness
// convention as Falchi.
//
// TODO(blocked: no numeric data — re-confirmed 2026-05): Lorenz still publishes
// only zone-color PNGs (each zone = ×3 brightness, sub-zones ×1.73 — too coarse
// to reverse-map reliably), with raw values available only by emailing the
// author. The "2024 data" downloadable from lightpollutionmap.info is in fact
// the WA/Falchi sky-brightness layer (see [NewFalchiProvider]) or raw VIIRS
// VJ146A4 radiance (see [NewVIIRSProvider] / [NewVIIRSHDF5Provider]) — not a
// distinct numeric LPA sky-brightness grid. Reverse-mapping the PNG palette
// would fabricate precision the source does not carry, so this loader is
// intentionally unimplemented. If you obtain a genuine numeric grid from the
// author, decode it to a [Grid] (mcd/m²) and use [NewGridProvider].
func NewLorenzProvider(_ io.ReaderAt) (skybrightness.SQMProvider, error) {
	return nil, ErrLorenzNoNumericData
}
