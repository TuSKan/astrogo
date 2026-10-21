package keplerlab_test

import (
	"math/big"

	"github.com/TuSKan/astrogo/internal/keplerlab"
)

// The oracle lives in oracle.go, shared with ephemeris/kepler's contract test;
// these are the names the experiments here were written with.
const (
	prec       = keplerlab.Prec
	reducePrec = keplerlab.ReducePrec
)

var (
	pi               = keplerlab.Pi
	twoPi            = keplerlab.TwoPi
	machinPi         = keplerlab.MachinPi
	agmPi            = keplerlab.AGMPi
	reduce           = keplerlab.Reduce
	sinCos           = keplerlab.SinCos
	keplerResidual   = keplerlab.Residual
	solveExact       = keplerlab.SolveExact
	exactStateFromE  = keplerlab.StateFromE
	stateSensitivity = keplerlab.Sensitivity
	errorOf          = keplerlab.ErrorOf
	absErr           = keplerlab.AbsErr
	norm             = keplerlab.Norm
	meanAnomalies    = keplerlab.MeanAnomalies
	eccentricities   = keplerlab.Eccentricities()
)

func newF(v float64) *big.Float { return new(big.Float).SetPrec(prec).SetFloat64(v) }

func newI(v int64) *big.Float { return new(big.Float).SetPrec(prec).SetInt64(v) }
