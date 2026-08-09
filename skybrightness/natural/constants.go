package natural

import "github.com/TuSKan/astrogo/skybrightness"

// FalchiNaturalZenithLuminance is the natural (non-artificial) zenith sky
// luminance Falchi et al. (2016), Sci. Adv. 2, e1600377 uses as the World
// Atlas 2015's natural background — 0.171168465 mcd/m^2, equivalent to
// 22.0 V mag/arcsec^2. Carried verbatim from astrogo v1's
// skybrightness.NaturalZenithMcdM2, converted from mcd/m^2 to cd/m^2 for
// this package's LuminanceCdM2 type. Model-dependent (a specific
// atlas's own natural-background assumption, not a universal physical
// constant), so it lives here rather than in package constants — see
// docs/skybrightness.md §3's photometric-constants table.
const FalchiNaturalZenithLuminance skybrightness.LuminanceCdM2 = 0.171168465e-3
