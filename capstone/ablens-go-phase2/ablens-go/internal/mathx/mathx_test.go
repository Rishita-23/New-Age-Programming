package mathx

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want, rel float64) {
	t.Helper()
	if math.Abs(got-want) > rel*math.Abs(want)+1e-300 {
		t.Errorf("%s: got %.12g want %.12g", name, got, want)
	}
}

func TestNormal(t *testing.T) {
	near(t, "p0.5", NormalTwoSidedP(0.5), 0.6170750774519738, 1e-10)
	near(t, "p1.96", NormalTwoSidedP(1.96), 0.04999579029644087, 1e-10)
	near(t, "p3.5", NormalTwoSidedP(3.5), 0.00046525815807105003, 1e-10)
	near(t, "p8", NormalTwoSidedP(8), 1.244192114854348e-15, 1e-8)
	near(t, "q975", NormalQuantile(0.975), 1.959963984540054, 1e-10)
	near(t, "q995", NormalQuantile(0.995), 2.5758293035489004, 1e-10)
}

func TestStudentT(t *testing.T) {
	near(t, "t2,10", StudentTTwoSidedP(2.0, 10), 0.07338803477074037, 1e-9)
	near(t, "t1.5,5.5", StudentTTwoSidedP(1.5, 5.5), 0.1886769689070986, 1e-9)
	near(t, "t4.2,120.3", StudentTTwoSidedP(4.2, 120.3), 5.14701215317523e-05, 1e-8)
	near(t, "t0.3,2000.7", StudentTTwoSidedP(0.3, 2000.7), 0.764208321079293, 1e-9)
	near(t, "crit10", StudentTCritical(0.05, 10), 2.228138851986274, 1e-9)
	near(t, "crit57.3", StudentTCritical(0.05, 57.3), 2.002238268231336, 1e-9)
}

func TestChiSquare(t *testing.T) {
	near(t, "3.84,1", ChiSquareSF(3.84, 1), 0.05004352124870519, 1e-9)
	near(t, "10.83,1", ChiSquareSF(10.83, 1), 0.0009986863791802592, 1e-9)
	near(t, "0.5,1", ChiSquareSF(0.5, 1), 0.47950012218695337, 1e-9)
	near(t, "7,3", ChiSquareSF(7, 3), 0.07189777249646509, 1e-9)
	near(t, "25,4", ChiSquareSF(25, 4), 5.0309817823062075e-05, 1e-8)
}
