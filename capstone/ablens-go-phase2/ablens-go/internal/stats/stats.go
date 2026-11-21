package stats

import (
	"errors"
	"math"

	"ablens/internal/mathx"
)

type TestResult struct {
	Name        string  `json:"name"`
	Statistic   float64 `json:"statistic"`
	DF          float64 `json:"df,omitempty"`
	PValue      float64 `json:"p_value"`
	Diff        float64 `json:"diff"`
	CILow       float64 `json:"ci_low"`
	CIHigh      float64 `json:"ci_high"`
	Significant bool    `json:"significant"`
}

var ErrNoVariance = errors.New("zero variance: test is undefined")
var ErrTooFew = errors.New("not enough observations")

func TwoProportionZ(n1 int, x1 float64, n2 int, x2 float64, alpha float64) (TestResult, error) {
	if n1 < 1 || n2 < 1 {
		return TestResult{}, ErrTooFew
	}
	fn1, fn2 := float64(n1), float64(n2)
	p1, p2 := x1/fn1, x2/fn2
	pool := (x1 + x2) / (fn1 + fn2)
	se := math.Sqrt(pool * (1 - pool) * (1/fn1 + 1/fn2))
	if se == 0 {
		return TestResult{}, ErrNoVariance
	}
	z := (p2 - p1) / se
	seU := math.Sqrt(p1*(1-p1)/fn1 + p2*(1-p2)/fn2)
	zc := mathx.NormalQuantile(1 - alpha/2)
	d := p2 - p1
	p := mathx.NormalTwoSidedP(z)
	return TestResult{
		Name: "two-proportion z-test", Statistic: z, PValue: p, Diff: d,
		CILow: d - zc*seU, CIHigh: d + zc*seU, Significant: p < alpha,
	}, nil
}

func Welch(n1 int, m1, v1 float64, n2 int, m2, v2 float64, alpha float64) (TestResult, error) {
	if n1 < 2 || n2 < 2 {
		return TestResult{}, ErrTooFew
	}
	a, b := v1/float64(n1), v2/float64(n2)
	se := math.Sqrt(a + b)
	if se == 0 {
		return TestResult{}, ErrNoVariance
	}
	t := (m2 - m1) / se
	df := (a + b) * (a + b) / (a*a/float64(n1-1) + b*b/float64(n2-1))
	p := mathx.StudentTTwoSidedP(t, df)
	tc := mathx.StudentTCritical(alpha, df)
	d := m2 - m1
	return TestResult{
		Name: "Welch t-test", Statistic: t, DF: df, PValue: p, Diff: d,
		CILow: d - tc*se, CIHigh: d + tc*se, Significant: p < alpha,
	}, nil
}

func ChiSquareGOF(observed []int, expected []float64) (chi float64, df int, p float64, err error) {
	if len(observed) != len(expected) || len(observed) < 2 {
		return 0, 0, 0, errors.New("observed and expected must have the same length (>= 2)")
	}
	for i, o := range observed {
		if expected[i] <= 0 {
			return 0, 0, 0, errors.New("expected counts must be positive")
		}
		d := float64(o) - expected[i]
		chi += d * d / expected[i]
	}
	df = len(observed) - 1
	return chi, df, mathx.ChiSquareSF(chi, df), nil
}
