// Package plan provides high-level tools for astronomical observation planning
// and robust scheduling.
//
// It includes the core observability evaluation API, a high-precision
// generalized EventSolver for identifying rise, set, and transit times, as well
// as complex geometric events (Conjunctions, Oppositions, and Greatest Elongations).
// It also features a full scheduling engine for generating observation timelines
// subject to constraints, slew transitions, and dynamical block prioritizations.
//
// # Finding what you need
//
// The exported surface is large; here's a task-oriented index instead of an
// alphabetical one:
//
//   - Site & observatory setup — [NewSite], [Site]
//   - Targets to observe — [Planet] (via [NewSun]...[NewPluto], [NewPlanet]),
//     [Star] (via [NewStar]), [Asteroid] (via [NewAsteroid]), [Comet] (via
//     [NewComet]), [DeepSkyObject] (via [NewDeepSkyObject]), [Satellite] (via
//     [NewSatellite]), [GenericBody] (via [NewGenericBody]), [FromCatalog];
//     all implement [Observable]/[MovingBody]/[MagnitudeComputer]
//   - Rise/set/twilight times — [SunEvents]/[SunriseSunset],
//     [MoonEvents]/[MoonriseMoonset], [TwilightEvents], [CivilDawnDusk],
//     [NauticalDawnDusk], [AstronomicalDawnDusk]
//   - Observability windows & scoring — [VisibilityEvents], [IsObservable],
//     [ScoreObservable], [RankObservables], [ObservableWindows],
//     [Constraint] ([Altitude], [Airmass], [Sun], [MoonSep]),
//     [LimitingMagnitudeConstraint], [ScoreObservableSky]
//   - Geometric events — [Conjunctions], [ConjunctionsEcliptic], [Appulses],
//     [Oppositions], [GreatestElongations], [FullMoonOppositions]
//   - Moon phases, seasons, eclipses — [MoonPhases], [MoonIllumination],
//     [Seasons], [Apsides], [LunarEclipses], [SolarEclipses], [NextNewMoon],
//     [NextFullMoon]
//   - Lunar crescent visibility — [NewCrescentParams]/[CrescentParams], whose
//     methods (Yallop, Odeh, Qureshi, Fotheringham, Danjon, MABIMS1995,
//     MABIMS2021, and more) implement the 20 published criteria;
//     [CrescentParams.EvaluateAll] runs all of them at once into a
//     [CrescentResult]
//   - Scheduling — [Scheduler]/[NewScheduler], [Strategy]
//     ([GreedyStrategy]/[PriorityStrategy]/[SwapOptimizedStrategy]),
//     [TransitionModel], [Block]/[Schedule]
//   - Satellite passes — [SatellitePasses], [LookAngle]
//   - Low-level event solving (for custom event families) — [EventSolver],
//     [Solver]/[DefaultSolver], [CrossesTarget]/[CrossesIncreasing]
//
// # Refinement
//
// The EventSolver uses a two-stage approach: a coarse sampling pass followed
// by numerical refinement using bisection (for zero-crossing boundaries) and
// golden-section search (for local maxima/extrema). This provides high temporal
// accuracy (e.g., 1 second) without requiring analytical closed-form solutions.
package plan
