package result

import "ablens/internal/stats"

type SRM struct {
	Observed  [2]int     `json:"observed"`
	Expected  [2]float64 `json:"expected"`
	ChiSquare float64    `json:"chi_square"`
	PValue    float64    `json:"p_value"`
	Flagged   bool       `json:"flagged"`
}

type Quality struct {
	TotalRows         int            `json:"total_rows"`
	ValidRows         int            `json:"valid_rows"`
	MalformedRows     int            `json:"malformed_rows"`
	MalformedReasons  map[string]int `json:"malformed_reasons,omitempty"`
	UnknownVariant    int            `json:"unknown_variant_rows"`
	DuplicateEvents   int            `json:"duplicate_event_rows"`
	CrossVariantUsers int            `json:"cross_variant_users"`
	CrossVariantRows  int            `json:"cross_variant_rows_dropped"`
	SRM               SRM            `json:"srm"`
}

type VariantSummary struct {
	Name     string  `json:"name"`
	N        int     `json:"n"`
	Mean     float64 `json:"mean"`
	Variance float64 `json:"variance"`
}

type Bootstrap struct {
	Iterations int     `json:"iterations"`
	Workers    int     `json:"workers"`
	CILow      float64 `json:"ci_low"`
	CIHigh     float64 `json:"ci_high"`
	StdErr     float64 `json:"std_err"`
	Seconds    float64 `json:"seconds"`
}

type MetricResult struct {
	Metric    string            `json:"metric"`
	Kind      string            `json:"kind"`
	Control   VariantSummary    `json:"control"`
	Treatment VariantSummary    `json:"treatment"`
	AbsLift   float64           `json:"abs_lift"`
	RelLift   *float64          `json:"rel_lift,omitempty"`
	Test      *stats.TestResult `json:"test,omitempty"`
	TestError string            `json:"test_error,omitempty"`
	Bootstrap *Bootstrap        `json:"bootstrap,omitempty"`
}

type Result struct {
	Experiment    string             `json:"experiment"`
	Alpha         float64            `json:"alpha"`
	Quality       Quality            `json:"data_quality"`
	Metrics       []MetricResult     `json:"metrics"`
	Verdict       string             `json:"verdict"`
	VerdictReason string             `json:"verdict_reason"`
	Timings       map[string]float64 `json:"timings_seconds"`
}
