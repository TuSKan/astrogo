package fits

import (
	"errors"
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
	Cards []Card
	keys  map[string]int // maps keyword to index in Cards
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
	val := strings.TrimSpace(card.Value)
	if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
		val = val[1 : len(val)-1]
	}
	// Also trim trailing spaces that might be inside the quoted string if it was padded
	return strings.TrimRight(val, " "), nil
}

// GetInt returns the value of a keyword as an integer.
func (h *Header) GetInt(keyword string) (int, error) {
	card, err := h.Get(keyword)
	if err != nil {
		return 0, err
	}
	val := strings.TrimSpace(card.Value)
	return strconv.Atoi(val)
}

// GetFloat returns the value of a keyword as a float64.
func (h *Header) GetFloat(keyword string) (float64, error) {
	card, err := h.Get(keyword)
	if err != nil {
		return 0.0, err
	}
	val := strings.TrimSpace(card.Value)
	return strconv.ParseFloat(val, 64)
}

// ParseCard extracts the value and comment string from a raw 80-byte FITS card.
func ParseCard(raw []byte) Card {
	s := string(raw)
	if len(s) > 80 {
		s = s[:80]
	}

	card := Card{}

	if len(s) < 8 {
		card.Keyword = strings.TrimSpace(s)
		return card
	}

	card.Keyword = strings.TrimSpace(s[0:8])

	if len(s) == 8 || len(strings.TrimSpace(s[8:])) == 0 {
		return card
	}

	rest := s[8:]
	if strings.HasPrefix(rest, "= ") {
		rest = rest[2:]

		// Find the true value and comment split considering string literals
		inQuote := false
		valEnd := len(rest)

		for i := 0; i < len(rest); i++ {
			if rest[i] == '\'' {
				inQuote = !inQuote
			} else if rest[i] == '/' && !inQuote {
				valEnd = i
				break
			}
		}

		card.Value = strings.TrimSpace(rest[:valEnd])
		if valEnd < len(rest) {
			card.Comment = strings.TrimSpace(rest[valEnd+1:])
		}
	} else {
		// Not a standard assignment card (such as HISTORY or COMMENT)
		card.Comment = strings.TrimSpace(rest)
	}

	return card
}
