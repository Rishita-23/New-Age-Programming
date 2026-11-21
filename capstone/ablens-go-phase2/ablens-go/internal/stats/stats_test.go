package stats

import (
	"errors"
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol*math.Abs(want)+1e-12 {
		t.Errorf("%s: got %.12g want %.12g", name, got, want)
	}
}

func TestTwoProportionZ(t *testing.T) {
	r, err := TwoProportionZ(10000, 1000, 10000, 1100, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "z", r.Statistic, 2.3066347738170103, 1e-9)
	near(t, "p", r.PValue, 0.021075189273797826, 1e-8)
	near(t, "lo", r.CILow, 0.001504059131512114, 1e-7)
	near(t, "hi", r.CIHigh, 0.018495940868487874, 1e-7)
	if !r.Significant {
		t.Error("expected significant")
	}
}

func TestTwoProportionZNoVariance(t *testing.T) {
	if _, err := TwoProportionZ(100, 0, 100, 0, 0.05); !errors.Is(err, ErrNoVariance) {
		t.Errorf("got %v", err)
	}
}

func TestWelch(t *testing.T) {
	r, err := Welch(40, 10, 25, 55, 12, 36, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "t", r.Statistic, 1.7680809156636257, 1e-9)
	near(t, "df", r.DF, 91.21140862841163, 1e-9)
	near(t, "p", r.PValue, 0.08039111281622939, 1e-8)
	near(t, "lo", r.CILow, -0.24685998556197797, 1e-7)
	near(t, "hi", r.CIHigh, 4.2468599855619775, 1e-7)
	if r.Significant {
		t.Error("expected not significant")
	}
}

func TestWelchErrors(t *testing.T) {
	if _, err := Welch(1, 1, 1, 5, 2, 1, 0.05); !errors.Is(err, ErrTooFew) {
		t.Errorf("got %v", err)
	}
	if _, err := Welch(5, 1, 0, 5, 2, 0, 0.05); !errors.Is(err, ErrNoVariance) {
		t.Errorf("got %v", err)
	}
}

func TestChiSquareGOF(t *testing.T) {
	chi, df, p, err := ChiSquareGOF([]int{5200, 4800}, []float64{5000, 5000})
	if err != nil {
		t.Fatal(err)
	}
	near(t, "chi", chi, 16, 1e-12)
	near(t, "p", p, 6.334248366623988e-05, 1e-8)
	if df != 1 {
		t.Errorf("df=%d", df)
	}
	chi, df, p, _ = ChiSquareGOF([]int{300, 350, 350}, []float64{300, 350, 350})
	if chi != 0 || df != 2 || math.Abs(p-1) > 1e-12 {
		t.Errorf("perfect fit: chi=%v df=%d p=%v", chi, df, p)
	}
	if _, _, _, err := ChiSquareGOF([]int{1}, []float64{1}); err == nil {
		t.Error("expected error for single category")
	}
}
