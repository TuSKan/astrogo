package fits

import "errors"

// VerifyError classifies FITS standard violations.
type VerifyError struct {
	Keyword string
	Reason  string
}

func (e VerifyError) Error() string {
	return "fits verify: keyword " + e.Keyword + " " + e.Reason
}

// VerifyPrimaryHeader ensures the primary HDU header strictly follows FITS semantics.
// At minimum, it must contain SIMPLE=T, BITPIX, NAXIS in that exact sequence.
func VerifyPrimaryHeader(h *Header) error {
	if len(h.Cards) == 0 {
		return errors.New("fits verify: empty header")
	}

	// 1. First keyword must be SIMPLE.
	if h.Cards[0].Keyword != "SIMPLE" {
		return VerifyError{Keyword: h.Cards[0].Keyword, Reason: "must be SIMPLE for primary HDU"}
	}
	// Note: FITS says SIMPLE must equal 'T' (true).
	if h.Cards[0].Value != "T" {
		return VerifyError{Keyword: "SIMPLE", Reason: "must equal T"}
	}

	// 2. Second keyword must be BITPIX.
	if len(h.Cards) < 2 || h.Cards[1].Keyword != "BITPIX" {
		return VerifyError{Keyword: "BITPIX", Reason: "must immediately follow SIMPLE"}
	}

	// 3. Third keyword must be NAXIS.
	if len(h.Cards) < 3 || h.Cards[2].Keyword != "NAXIS" {
		return VerifyError{Keyword: "NAXIS", Reason: "must immediately follow BITPIX"}
	}

	return nil
}
