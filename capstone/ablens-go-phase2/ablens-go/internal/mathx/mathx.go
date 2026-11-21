package mathx

import "math"

const (
	eps   = 3e-16
	fpmin = 1e-300
)

func NormalTwoSidedP(z float64) float64 {
	return math.Erfc(math.Abs(z) / math.Sqrt2)
}

func NormalQuantile(p float64) float64 {
	return math.Sqrt2 * math.Erfinv(2*p-1)
}

func betacf(a, b, x float64) float64 {
	qab, qap, qam := a+b, a+1, a-1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < fpmin {
		d = fpmin
	}
	d = 1 / d
	h := d
	for m := 1; m <= 1000; m++ {
		fm := float64(m)
		m2 := 2 * fm
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		h *= d * c
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}

func RegIncBeta(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	lab, _ := math.Lgamma(a + b)
	bt := math.Exp(lab - la - lb + a*math.Log(x) + b*math.Log1p(-x))
	if x < (a+1)/(a+b+2) {
		return bt * betacf(a, b, x) / a
	}
	return 1 - bt*betacf(b, a, 1-x)/b
}

func StudentTTwoSidedP(t, df float64) float64 {
	return RegIncBeta(df/2, 0.5, df/(df+t*t))
}

func StudentTCritical(alpha, df float64) float64 {
	lo, hi := 0.0, 1.0
	for StudentTTwoSidedP(hi, df) > alpha {
		hi *= 2
		if hi > 1e12 {
			break
		}
	}
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if StudentTTwoSidedP(mid, df) > alpha {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

func gammaSeries(a, x float64) float64 {
	lg, _ := math.Lgamma(a)
	ap := a
	sum := 1 / a
	del := sum
	for n := 0; n < 2000; n++ {
		ap++
		del *= x / ap
		sum += del
		if math.Abs(del) < math.Abs(sum)*eps {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-lg)
}

func gammaCF(a, x float64) float64 {
	lg, _ := math.Lgamma(a)
	b := x + 1 - a
	c := 1 / fpmin
	d := 1 / b
	h := d
	for i := 1; i <= 2000; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = b + an/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-lg) * h
}

func GammaQ(a, x float64) float64 {
	if x <= 0 {
		return 1
	}
	if x < a+1 {
		return 1 - gammaSeries(a, x)
	}
	return gammaCF(a, x)
}

func ChiSquareSF(x float64, k int) float64 {
	return GammaQ(float64(k)/2, x/2)
}
