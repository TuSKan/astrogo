package fits

import (
	"fmt"
	"strings"

	"github.com/TuSKan/astrogo/wcs"
)

// ExtractWCS dynamically translates FITS standard header layouts structurally matching
// coordinate metrics arrays natively mapping the resulting abstractions into `wcs.WCS`.
func ExtractWCS(h *Header) (*wcs.WCS, error) {
	naxis, err := h.GetInt("NAXIS")
	if err != nil {
		return nil, fmt.Errorf("fits/wcs: header missing mandatory NAXIS keyword")
	}
	if naxis <= 0 {
		return nil, fmt.Errorf("fits/wcs: header defines mathematically 0-dimensional plane")
	}

	w := wcs.New(naxis)

	// Pull static baseline Coordinate properties dynamically building axes
	for i := 1; i <= naxis; i++ {
		idx := i - 1

		// CTYPE handles explicit celestial bindings configurations handling standard string stripping formats naturally
		ctype, _ := h.GetString(fmt.Sprintf("CTYPE%d", i))
		w.CTYPE[idx] = strings.TrimSpace(ctype)

		// Reference center absolute mapping metrics
		crval, err := h.GetFloat(fmt.Sprintf("CRVAL%d", i))
		if err == nil {
			w.CRVAL[idx] = crval
		}

		// Reference pixel natively represented inside FITS math conventions over 1-indexed structures.
		crpix, err := h.GetFloat(fmt.Sprintf("CRPIX%d", i))
		if err == nil {
			w.CRPIX[idx] = crpix
		}

		// Scales and delta margins mathematically standardizing bounding increments.
		// Defaults fallback natively matching exactly 1.0 bounding constraints per standard formats.
		cdelt, err := h.GetFloat(fmt.Sprintf("CDELT%d", i))
		if err != nil {
			cdelt = 1.0
		}
		w.CDELT[idx] = cdelt
	}

	// Pull Matrix transformation natively scanning PCi_j offsets matching skew/rotation scaling grids natively
	for i := 1; i <= naxis; i++ {
		for j := 1; j <= naxis; j++ {
			val, err := h.GetFloat(fmt.Sprintf("PC%d_%d", i, j))
			if err == nil {
				w.PC[i-1][j-1] = val
			} else {
				// Matrix natively defaults scaling properties natively inside strictly diagonal mapping arrays.
				if i == j {
					w.PC[i-1][j-1] = 1.0
				} else {
					w.PC[i-1][j-1] = 0.0
				}
			}
		}
	}

	return w, nil
}
