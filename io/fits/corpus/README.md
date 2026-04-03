# Astropy Interoperability Corpus

This directory is designed to house verified FITS samples generated natively by `astropy.io.fits` to validate AstroGo's absolute byte-for-byte correctness and interoperability.

## Structure
- `binary_tables/`: Contains exact `BINTABLE` schemas.
- `images/`: Float32, Int16, and other ND-tensors mapping payload scales.
- `compressed/`: Native `.gz` wrapped FITS streams.

## Usage
When running tests, AstroGo will execute `OpenMmap`, `Open`, and `Read` across these files verifying strict structural integrity against standard outputs, proving interoperability stability.
