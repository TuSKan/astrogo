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
// # Phase 0
//
// This is the spectral foundation only. It ships no [Component]
// implementations at all: a [Model] with no components returns a zero
// radiance and says so through [Quality]. Nothing here should be read as a
// physical sky prediction yet.
//
// The scientific components arrive in the order set out in
// docs/skybrightness.md: atmosphere, then artificial clear sky
// (Kocifaj, Bara & Falchi 2022), clouds (Kocifaj, Falchi & Kundracik
// 2025), the natural sky (GAMBONS), and the Moon (Jones 2013 with
// Kieffer-Stone reflectance and Winkler 2022 scattering). Each lands only
// once its primary literature is in hand; none is approximated to make a
// phase look finished.
package skybrightness
