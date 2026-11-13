// Package plan answers the questions an observing session asks: what can be
// seen from here tonight, when a target rises, transits and sets, which
// events are coming, and in what order to observe.
//
// Most work starts with a [Site] and one or more targets, each an
// [Observable]. From there, [VisibleTonight] answers what is up,
// [ObservableWindows] when a target meets a set of constraints, the event
// functions what happens and when, and a [Scheduler] the order to observe in.
// The sections below group the package by task; the index after them lists
// it alphabetically.
//
// # Sites
//
// [NewSite] builds a [Site] from a geodetic location; [NewSiteEarthLocation]
// takes plain latitude, longitude and height, and [NewSiteEarthAddress]
// geocodes a free-text address. Well-known observatories come from
// [KnownSiteNames] and [NewKnownSite], and the Minor Planet Center's
// observatory codes from [MPCObservatories] and [NewMPCSite]. A [SiteOption]
// adds a local horizon ([WithHorizon], [WithHorizonProfile] and a
// [HorizonProfile], [WithHorizonRefraction]), a time zone ([WithTimeZone]),
// an MPC code ([WithMPCCode]) or aliases ([WithSiteAliases]).
//
// # Targets
//
// Every target implements [Observable]; one that moves also implements
// [MovingBody], and one with a brightness model [MagnitudeComputer]. A
// moving target is placed by the ephemeris provider its constructor takes.
//
//   - Solar-system bodies: [NewSun], [NewMoon], [NewMercury] through
//     [NewPluto], or [NewPlanet] by name, each a [Planet]; [NewPlanetaryMoon]
//     for a planet's satellites; [NewAsteroid], [NewComet], and
//     [NewGenericBody] for any other body a provider places.
//   - Fixed targets: [NewStar], with proper motion, parallax and radial
//     velocity as [StarOption]s; [NewDeepSkyObject]; [NewConstellation].
//   - Earth satellites: [NewSatellite].
//   - Meteor showers: [NewMeteorShower] and [MeteorShowerNames].
//   - From a catalog query: [FromCatalog].
//
// Every target's GetDetails method gives its [TargetDetails] at an instant;
// [RadialVelocity] and [AngularDiameter] give a target's radial velocity and
// angular size.
//
// # What is up tonight
//
// [VisibleTonight] lists the [VisibleObject]s above a site between dusk and
// dawn, tuned by [VisibleTonightOption]s such as [WithMinAltitude] and
// [WithPlanetaryMoons]. [IsCircumpolar] and [IsNeverUp] answer the two
// cases that need no search.
//
// # Constraints, windows and scores
//
// A [Constraint] decides whether a target is observable at an instant. The
// built-in ones are [Altitude], [Airmass], [Horizon], [Sun], [SunSep],
// [MoonSep], [MoonIllum], [GalacticLatitude], [TimeWindow] and
// [LimitingMagnitudeConstraint], which needs a [SkyDepth].
//
//   - At one instant: [IsObservable] gives an [Evaluation] with each
//     constraint's [Result].
//   - Over a span: [ObservableWindows] gives the [Window]s that meet every
//     constraint; [VisibleIntervals], [TransitEstimate],
//     [MaxAltitudeInWindow] and [Find] answer narrower questions, and
//     [Union], [Intersect], [Subtract] and [TotalDuration] combine windows.
//   - Ranking: a [Scorer], configured by a [ScoreConfig], scores a target,
//     and [RankObservables] orders many.
//
// # Rise, set and twilight
//
// [SunEvents] and [SunriseSunset], [MoonEvents] and [MoonriseMoonset], and
// [TwilightEvents] for each [TwilightKind]; [CivilDawnDusk],
// [NauticalDawnDusk] and [AstronomicalDawnDusk] answer the common case
// directly. [DayEvents] gives a target's rise, set and transit within one
// local calendar day, and [Episode] the whole continuous span it is up
// around an interval.
//
// # Geometric events
//
// [Conjunctions], [ConjunctionsEcliptic], [Appulses], [Oppositions],
// [FullMoonOppositions] and [GreatestElongations]; on the ground,
// [SubsolarPoint], [SublunarPoint] and the [Terminator].
//
// # Moon, seasons, apsides and eclipses
//
// [MoonPhases], [NextNewMoon] and [NextFullMoon]; [MoonIllumination],
// [MoonElongation] and [MoonPhaseFraction] at an instant. [Seasons],
// [Apsides], [LunarEclipses] and [SolarEclipses] list their events over a
// span.
//
// # Lunar crescent visibility
//
// [CrescentVisibility] evaluates an evening, each criterion in the
// convention it was defined in, into a [CrescentResult] carrying every
// answer with the quantities it read; [CrescentParams]' methods (Yallop,
// Odeh, Qureshi, Fotheringham, Danjon, MABIMS2021, and more) implement the
// 18 published criteria.
//
// # Satellite passes
//
// [SatellitePasses] finds a satellite's [SatellitePass]es over a site, and
// [LookAngle] where it is at one instant.
//
// # Scheduling
//
// A [Scheduler], from [NewScheduler], places [Block]s into a [Schedule] of
// [ScheduledBlock]s, reporting the rest as [UnscheduledBlock]s. Its
// [Strategy] decides the order ([GreedyStrategy], [PriorityStrategy],
// [SwapOptimizedStrategy]), and its [TransitionModel] what moving between
// two blocks costs ([BasicTransitionModel]).
//
// # Custom events
//
// [NewEventSolver] finds the [Event]s an [EventSpec] describes, for event
// kinds the functions above do not cover. [Solver], [DefaultSolver],
// [CrossesTarget] and [CrossesIncreasing] are the root finding beneath it.
//
// The solver samples coarsely, then refines: bisection for a boundary,
// golden-section search for an extremum. That reaches about a second of
// accuracy without a closed-form solution.
//
// # Errors
//
// Errors a caller can act on wrap one of the package's sentinels, for
// errors.Is: bad input ([ErrInvalidHorizon], [ErrReversedInterval],
// [ErrNonFiniteInterval], [ErrStepNotPositive], [ErrSlewTime]), a name not
// found ([ErrUnknownSite], [ErrUnknownMeteorShower],
// [ErrUnknownPlanetaryMoon], [ErrGeocodeNoResult]), an event that does not
// occur ([ErrEventNotFound], [ErrNoTwilight]), a solver that failed
// ([ErrNoConvergence], [ErrNonFiniteEvaluation]), and a result that is only
// partial ([ErrIncomplete]). The index lists them all under Variables.
//
// # Related packages
//
//   - [github.com/TuSKan/astrogo/fits/fitsplan] builds a [Site] and targets
//     from FITS header metadata.
//   - [github.com/TuSKan/astrogo/skybrightness/skyplan] supplies the
//     [SkyDepth] that [LimitingMagnitudeConstraint] needs.
//   - [github.com/TuSKan/astrogo/catalog] resolves names to the targets
//     [FromCatalog] turns into [Observable]s.
package plan
