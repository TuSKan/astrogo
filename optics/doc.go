// Package optics computes the practical numbers an observer needs to plan
// a session with a specific telescope/eyepiece/camera combination:
// magnification, true and apparent field of view, exit pupil, useful
// magnification range, resolving power, and pixel scale.
//
// This is pure equipment-optics arithmetic — no astrometry, no ephemeris,
// no network access. It sits at the same layer as angle/vector/unit/
// constants (see CLAUDE.md's Architecture section): a top-level primitive
// package, not a plan subpackage, since plan is orchestration and has no
// business depending on or being depended on by equipment math.
//
// Telescope and Eyepiece use validating constructors with unexported
// fields (mirroring plan.Site's pattern) so a zero-value Telescope{} or
// Eyepiece{} can never silently divide by zero or produce a NaN/Inf
// result — NewTelescope/NewEyepiece reject non-positive dimensions
// outright. Sensor is a plain struct with exported fields; it has no
// invalid combination to construct-time-validate beyond what each
// computation already handles.
//
// Typical use:
//
//	scope, _ := optics.NewTelescope(200, 2000)               // 200mm f/10
//	eyepiece, _ := optics.NewEyepiece(25, angle.Deg(68))      // 25mm, 68° AFOV
//	mag := scope.Magnification(eyepiece)                      // 80x
//	tfov := scope.TrueFOV(eyepiece)                            // 0.85°
package optics
