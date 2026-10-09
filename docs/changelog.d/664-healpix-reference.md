---
type: Added
pr: 664
---
**`coord.HEALPix`'s NESTED indexing is held to an independent implementation.** `TestHEALPixMatchesAstropyHEALPix` checks `PixelOf` against astropy-healpix 2.0.1 at 32 directions across all twelve base faces, at nside 256 and 4096. The existing tests were self-consistency checks, which a scheme with its x and y bits interleaved the other way passes in full.
