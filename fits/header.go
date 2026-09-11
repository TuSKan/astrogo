package fits

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrKeyNotFound indicates a keyword was not found in the Header.
var ErrKeyNotFound = errors.New("fits: keyword not found")

// Card represents a single 80-byte FITS header record.
type Card struct {
	Keyword string
	Value   string
	Comment string
}

// Header is a collection of FITS records, providing ordered storage and type-safe access.
type Header struct {
	keys  map[string]int
	Cards []Card
}

// NewHeader initializes an empty FITS Header.
func NewHeader() *Header {
	return &Header{
		Cards: make([]Card, 0),
		keys:  make(map[string]int),
	}
}

// Append adds a new card to the header.
func (h *Header) Append(c Card) {
	kw := strings.ToUpper(strings.TrimSpace(c.Keyword))
	if idx, exists := h.keys[kw]; exists {
		h.Cards[idx] = c // Override existing
	} else {
		h.keys[kw] = len(h.Cards)
		h.Cards = append(h.Cards, c)
	}
}

// Get finds a card by keyword.
func (h *Header) Get(keyword string) (Card, error) {
	kw := strings.ToUpper(strings.TrimSpace(keyword))

	idx, exists := h.keys[kw]
	if !exists {
		return Card{}, ErrKeyNotFound
	}

	return h.Cards[idx], nil
}

// GetString returns the value of a keyword as a string.
func (h *Header) GetString(keyword string) (string, error) {
	card, err := h.Get(keyword)
	if err != nil {
		return "", err
	}

	// unquote also collapses the doubled quote FITS uses to escape a literal
	// one, so a value written as 'O''Brien' reads back as O'Brien rather than
	// as the text a writer would have to double again.
	return unquote(strings.TrimSpace(card.Value)), nil
}

// GetInt returns the value of a keyword as an integer.
func (h *Header) GetInt(keyword string) (int, error) {
	card, err := h.Get(keyword)
	if err != nil {
		return 0, err
	}

	val := strings.TrimSpace(card.Value)

	v, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("fits: parse int %q: %w", keyword, err)
	}

	return v, nil
}

// GetFloat returns the value of a keyword as a float64.
func (h *Header) GetFloat(keyword string) (float64, error) {
	card, err := h.Get(keyword)
	if err != nil {
		return 0.0, err
	}

	val := strings.TrimSpace(card.Value)

	v, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, fmt.Errorf("fits: parse float %q: %w", keyword, err)
	}

	return v, nil
}

// ParseCard extracts the value and comment string from a raw 80-byte FITS card.
//
// Three card shapes are recognised, which is every shape a header actually
// contains:
//
//   - A value card: keyword in columns 1-8, "= " in 9-10, then the value and
//     an optional " / comment".
//   - A HIERARCH card, whose keyword is longer than eight characters and runs
//     up to the first "=" — see [github.com/TuSKan/astrogo/fits] on the
//     conventions.
//   - A commentary card (COMMENT, HISTORY, or a blank keyword), whose whole
//     remainder is text.
//
// The value is returned as written, quotes included, so a card read here and
// written back by [Write] is the same card.
func ParseCard(raw []byte) Card {
	s := string(raw)
	if len(s) > CardSize {
		s = s[:CardSize]
	}

	// A HIERARCH keyword is longer than the eight-column field, so it has to
	// be recognised before the columns are trusted.
	if kw, rest, ok := parseHierarch(s); ok {
		value, comment := splitValueComment(rest)

		return Card{Keyword: kw, Value: value, Comment: comment}
	}

	card := Card{}

	if len(s) < keywordWidth {
		card.Keyword = strings.TrimSpace(s)

		return card
	}

	card.Keyword = strings.TrimSpace(s[0:keywordWidth])

	if len(s) == keywordWidth || strings.TrimSpace(s[keywordWidth:]) == "" {
		return card
	}

	rest := s[keywordWidth:]

	// CONTINUE carries a bare string in the value field with no "= ", and is
	// joined onto the preceding card by ReadHeader.
	if card.Keyword == continueKeyword {
		card.Value, card.Comment = splitValueComment(rest)

		return card
	}

	if !strings.HasPrefix(rest, valueIndicator) {
		// Not an assignment: COMMENT, HISTORY, or a blank keyword.
		card.Comment = strings.TrimSpace(rest)

		return card
	}

	card.Value, card.Comment = splitValueComment(rest[len(valueIndicator):])

	return card
}
