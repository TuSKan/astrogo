package fits

import (
	"fmt"
	"strings"

	"github.com/TuSKan/astrogo/coord"
)

// ExtractWCS dynamically translates FITS standard header layouts structurally matching
// coordinate metrics arrays natively mapping the resulting abstractions into `wcs.WCS`.
func ExtractWCS(h *Header) (*coord.WCS, error) {
	naxis, err := h.GetInt("NAXIS")
	if err != nil {
		return nil, fmt.Errorf("fits/wcs: header missing mandatory NAXIS keyword")
	}
	if naxis <= 0 {
		return nil, fmt.Errorf("fits/wcs: header defines mathematically 0-dimensional plane")
	}

	crpix := make([]float64, naxis)
	crval := make([]float64, naxis)
	cdelt := make([]float64, naxis)
	ctype := make([]string, naxis)
	pc := make([][]float64, naxis)

	for i := 1; i <= naxis; i++ {
		idx := i - 1

		c, _ := h.GetString(fmt.Sprintf("CTYPE%d", i))
		ctype[idx] = strings.TrimSpace(c)

		if v, err := h.GetFloat(fmt.Sprintf("CRVAL%d", i)); err == nil {
			crval[idx] = v
		}

		if p, err := h.GetFloat(fmt.Sprintf("CRPIX%d", i)); err == nil {
			crpix[idx] = p
		}

		d, err := h.GetFloat(fmt.Sprintf("CDELT%d", i))
		if err != nil {
			d = 1.0
		}
		cdelt[idx] = d

		pc[idx] = make([]float64, naxis)
		for j := 1; j <= naxis; j++ {
			val, err := h.GetFloat(fmt.Sprintf("PC%d_%d", i, j))
			if err == nil {
				pc[idx][j-1] = val
			} else {
				if i == j {
					pc[idx][j-1] = 1.0
				}
			}
		}
	}

	w := coord.NewWCS(naxis)
	w.SetCTYPE(ctype)
	w.SetCRVAL(crval)
	w.SetCRPIX(crpix)
	w.SetCDELT(cdelt)
	w.SetPC(pc)

	return w, nil
}
