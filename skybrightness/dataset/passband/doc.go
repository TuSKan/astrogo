// Package passband is the IO-tier provider for skybrightness.PassbandSet:
// a versioned, checksummed bundle of real response curves, read from disk
// (OpenBundle) or fetched via remote.PassbandBundle (Remote). This is the
// only place a passband response curve ever touches the filesystem or
// network in this module — core skybrightness never tabulates a curve in
// Go source (docs/skybrightness.md §3), and no unit test in this module
// depends on a real bundle existing: core's tests use
// skybrightness.TopHat/Gaussian instead.
//
// Bundle format (a directory): manifest.json (schema below) plus one CSV
// per curve, two columns "wavelength_nm,response", header row required.
//
//	{
//	  "version": "passbands-v1",
//	  "curves": [
//	    {"id": "johnson.V", "file": "johnson/V.csv", "system": "Vega",
//	     "detector": "EnergyIntegrating", "checksum": "sha256:...",
//	     "vega_mean_flambda_w_m2_sr_nm": 1.23e-9, "vega_spectrum": "CALSPEC alpha_lyr_stis_011",
//	     "source": "Bessell (1990), PASP 102, 1181", "licence": "public domain"}
//	  ]
//	}
//
// No such bundle is shipped with this module (docs/skybrightness.md §13:
// passband curve licensing needs verification before publication — a
// Phase 0/1 checklist item, not assumed clear). OpenBundle/Remote are
// fully functional against a bundle a caller supplies or that a future
// release publishes at remote.PassbandBundle.
package passband
