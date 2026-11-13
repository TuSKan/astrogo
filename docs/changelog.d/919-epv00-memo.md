---
type: Changed
pr: 919
---
**A search that comes back to an instant no longer evaluates `Epv00` again for it.** One bounded table of 2048 evaluations, shared by every
provider, replaces the memo of the last one. Against #1110's shared base, the 14-night Sun and Moon forecast is 57% faster, oppositions
92%, twenty targets under the Sun and Moon constraints 56%, and every result is unchanged to the last bit.
