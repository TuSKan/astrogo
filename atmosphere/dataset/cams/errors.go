package cams

import "errors"

// Sentinel errors returned by this package. Match with errors.Is.
var (
	// ErrNoLevelDimension is returned by Var.ReadPlane/Var.At when a
	// non-zero level index is requested against a variable with no level
	// axis (e.g. lnsp, which is (time, latitude, longitude) only).
	ErrNoLevelDimension = errors.New("cams: variable has no level dimension")

	// ErrUnsupportedAxis is returned when a data variable's discovered
	// axis order (via its _Netcdf4Coordinates attribute) names a
	// dimension this reader does not recognize. CAMS global-analysis
	// files use exactly time/level/latitude/longitude; anything else is
	// outside this package's documented scope rather than silently
	// mishandled.
	ErrUnsupportedAxis = errors.New("cams: variable has an unrecognized axis")

	// ErrIndexOutOfRange is returned by Var.At for a lat/lon index outside
	// [0, dimension length).
	ErrIndexOutOfRange = errors.New("cams: index out of range")

	// ErrVariableNotFound is returned by File.Var when the file has no
	// variable by the requested name — real and expected, since tracer
	// availability is dataset/version-specific (docs/skybrightness.md
	// §8), not a decode failure. Check errors.Is(err, ErrVariableNotFound)
	// to distinguish "this file doesn't have that tracer" from a genuine
	// decode failure on a variable the file does have.
	ErrVariableNotFound = errors.New("cams: variable not found")

	// ErrUnexpectedAttributeType is returned when a known attribute
	// (_FillValue, missing_value, _Netcdf4Dimid, _Netcdf4Coordinates)
	// decodes to a Go type this reader does not know how to interpret —
	// a real, surfaced failure rather than a silently-ignored attribute.
	ErrUnexpectedAttributeType = errors.New("cams: attribute has an unexpected type")
)
