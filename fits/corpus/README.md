# Astropy Interoperability Corpus

This directory is designed to house verified FITS samples generated natively by `astropy.io.fits` to validate AstroGo's absolute byte-for-byte correctness and interoperability.

## Structure
- `binary_tables/`: Contains exact `BINTABLE` schemas.
- `images/`: Float32, Int16, and other ND-tensors mapping payload scales.
- `compressed/`: Native `.gz` wrapped FITS streams.

## Usage
When running tests, AstroGo will execute `OpenMmap`, `Open`, and `Read` across these files verifying strict structural integrity against standard outputs, proving interoperability stability.

## Benchmark Resources
For verifying large-file zero-copy allocations and high performance memory mapping, use the following 213MB payload image:
- **Hubble**: `https://registry.opendata.aws/mast-hst/`
- **Source API**: `https://mast.stsci.edu/api/v0.1/Download/file/?uri=mast:HST/product/j8pu0y010_drc.fits`
- **AWS S3**: `s3://stpubdata/hst/public/j8pu/j8pu0y010/j8pu0y010_drc.fits`

*Note: For testing, this is automatically ignored by git via `.gitignore` when placed in the root `data/` or `corpus/` directories.*
