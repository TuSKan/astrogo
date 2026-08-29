// Package skybrightness predicts the spectral radiance of the night sky
// an instrument will actually see.
//
//	L_lambda(lambda, direction, observer, time, atmosphere)   W m^-2 sr^-1 nm^-1
//
// It answers one question: given a place, a time, a viewing direction, an
// atmospheric state, a cloud field, a surrounding artificial-light
// environment and an observing instrument, what spectral sky background
// does that instrument see, and how uncertain is the prediction?
//
// # Spectral radiance is the primary quantity
//
// Everything is computed as spectral radiance and stays spectral until
// the moment a caller asks for something else. Surface brightness in
// mag/arcsec^2, an SQM reading, luminance, a photon rate and a detector
// electron rate are all *projections* of that one spectral state, produced
// by [magnitude] and [optics] respectively. They are never the internal
// representation, because a model can reproduce a correct V magnitude with
// an entirely wrong spectrum, and every instrument projection downstream
// would then be wrong.
//
// Radiance is linear and additive; magnitudes are logarithmic and are not.
// Components sum in radiance space, and the conversion happens once, at
// the end.
//
// # What this package owns, and what it does not
//
// This package owns radiance transport: the [Scene], the [Component]
// contract, the [Model] that sums components, and the uncertainty, quality
// and provenance attached to a [Estimate].
//
// It deliberately owns nothing else. Atmospheric physics — Rayleigh and
// aerosol scattering, molecular absorption, transmission, vertical
// profiles, cloud optical properties — lives in [atmosphere]. Passbands
// and magnitude systems live in [magnitude]. Instrument throughput and
// detector rates live in [optics]. Spectral quantity types and the shared
// wavelength axis live in [unit]. Geometry, ephemerides and time scales
// come from [coord], [ephemeris] and [time]. A capability that belongs to
// one of those packages is added there, not duplicated here.
//
// # Getting a number
//
// [NewPreset] builds a model from a named configuration, and
// [github.com/TuSKan/astrogo/skybrightness/dataset.Inputs] gathers the
// reference data it reads:
//
//	ids, size := dataset.Endpoints(skybrightness.GAMBONSWeb)
//	remote.EnableDownloads(size, ids...)
//
//	in, err := dataset.Inputs(ctx, dataset.Spec{Preset: skybrightness.GAMBONSWeb})
//	model, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, in)
//
//	est, err := model.Estimate(ctx, skybrightness.Query{
//		Scene:     scene,
//		Direction: coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
//		Grid:      in.Grid,
//		Fidelity:  skybrightness.Standard,
//	})
//	mag, err := est.SurfaceBrightness(in.Band, magnitude.Vega)
//
// The scene carries the observer, the instant and the atmosphere, and its
// atmosphere must carry the preset's own transfer — [Preset.DiffuseKappa] and
// [Preset.MultipleScattering] — at the fidelity [Preset.Fidelity] names. A
// scene that disagrees is rejected rather than answered, because a sky
// evaluated under the wrong transfer is smooth, positive and wrong.
//
// examples/18_sky_brightness is the whole thing, runnable.
//
// # What is implemented
//
// Five components make up the natural sky: [IntegratedStarlight],
// [DiffuseGalacticLight], [ZodiacalLight], [Airglow] and
// [ExtragalacticBackground]. Two more are the module's own additions to
// GAMBONS, which models a moonless natural sky and has no term for either:
// [ScatteredMoonlight] (ROLO reflectance with single-scattering transfer) and
// [ArtificialSkyglow] (Kocifaj, Bara & Falchi 2022 over a caller's ground
// emitters).
//
// Four presets configure them: [GAMBONSWeb] and [GAMBONSFull] reproduce the
// published model as its web service and its paper compute it, [NaturalSky] is
// the same physics at Duriscoe's transfer factor, and [Observatory] is this
// module's own model — the only one not trying to be somebody else.
//
// The natural sky is validated against GAMBONS' published all-sky export at
// −0.03 mag with airglow removed from both sides. What remains unbuilt is
// clouds (Kocifaj, Falchi & Kundracik 2025) and an absolute calibration for
// artificial skyglow, whose directional structure is meaningful and whose
// overall scale is not; docs/skybrightness.md §16 records why, and it is a
// literature gap rather than an unwritten function.
//
// A [Model] with no components is still legal and returns zero radiance,
// saying so through [Quality] rather than pretending to be a dark sky.
//
// # Concurrency and I/O
//
// Evaluation performs no I/O and no network access: every dataset is resolved
// by the provider layer under skybrightness/dataset and handed in through the
// scene, which is enforced behaviourally by a test that evaluates identically
// under remote.SetOffline and structurally by an import check. A model is
// read-only once built and may be evaluated from several goroutines; the
// spectral buffers a caller passes in are the caller's to keep separate.
package skybrightness
