package agg

import (
	"fmt"

	"ablens/internal/model"
)

type Acc struct {
	N    int
	Mean float64
	M2   float64
}

func (a *Acc) Add(x float64) {
	a.N++
	d := x - a.Mean
	a.Mean += d / float64(a.N)
	a.M2 += d * (x - a.Mean)
}

func (a Acc) Variance() float64 {
	if a.N < 2 {
		return 0
	}
	return a.M2 / float64(a.N-1)
}

type Dataset struct {
	Metrics []string
	Samples [2][][]float64
	Acc     [2][]Acc
}

func value(o model.Obs, metric string) (float64, error) {
	switch metric {
	case "converted":
		return float64(o.Converted), nil
	case "revenue":
		return o.Revenue, nil
	}
	return 0, fmt.Errorf("unknown metric %q", metric)
}

func Build(clean []model.Obs, metrics []string) (*Dataset, error) {
	d := &Dataset{Metrics: metrics}
	for v := 0; v < 2; v++ {
		d.Samples[v] = make([][]float64, len(metrics))
		d.Acc[v] = make([]Acc, len(metrics))
	}
	for _, o := range clean {
		v := o.Variant
		for i, m := range metrics {
			x, err := value(o, m)
			if err != nil {
				return nil, err
			}
			d.Samples[v][i] = append(d.Samples[v][i], x)
			d.Acc[v][i].Add(x)
		}
	}
	return d, nil
}
