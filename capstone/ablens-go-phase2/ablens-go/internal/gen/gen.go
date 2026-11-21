package gen

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"strconv"
	"time"
)

type Options struct {
	Rows        int
	Seed        uint64
	Format      string
	BaseRate    float64
	Lift        float64
	MeanRevenue float64
	RevenueLift float64
	Split       float64
	DupRate     float64
	CrossRate   float64
	BadRate     float64
}

func Defaults() Options {
	return Options{
		Rows: 100000, Seed: 1, Format: "csv", BaseRate: 0.10, Lift: 0.01,
		MeanRevenue: 20, RevenueLift: 0.05, Split: 0.5,
	}
}

func (o Options) validate() error {
	switch {
	case o.Rows < 1:
		return errors.New("rows must be >= 1")
	case o.Format != "csv" && o.Format != "jsonl":
		return errors.New("format must be csv or jsonl")
	case o.BaseRate < 0 || o.BaseRate+o.Lift > 1 || o.BaseRate+o.Lift < 0:
		return errors.New("base rate and lift must keep conversion rates within [0,1]")
	case o.Split <= 0 || o.Split >= 1:
		return errors.New("split must be between 0 and 1")
	case o.DupRate < 0 || o.CrossRate < 0 || o.BadRate < 0 || o.DupRate+o.CrossRate+o.BadRate > 1:
		return errors.New("dup, cross and bad rates must be non-negative and sum to at most 1")
	}
	return nil
}

var epoch = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func Generate(w io.Writer, o Options) error {
	if err := o.validate(); err != nil {
		return err
	}
	rng := rand.New(rand.NewPCG(o.Seed, 0xAB1E))
	bw := bufio.NewWriterSize(w, 1<<20)
	if o.Format == "csv" {
		if _, err := bw.WriteString("event_id,user_id,variant,timestamp,converted,revenue\n"); err != nil {
			return err
		}
	}
	var prev string
	emit := func(id, user, variant, ts, conv, rev string) error {
		var line string
		if o.Format == "csv" {
			line = id + "," + user + "," + variant + "," + ts + "," + conv + "," + rev + "\n"
		} else {
			line = fmt.Sprintf("{\"event_id\":%q,\"user_id\":%q,\"variant\":%q,\"timestamp\":%q,\"converted\":%s,\"revenue\":%s}\n",
				id, user, variant, ts, conv, orZero(rev))
		}
		prev = line
		_, err := bw.WriteString(line)
		return err
	}
	for i := 0; i < o.Rows; i++ {
		id := "e" + strconv.Itoa(i)
		user := "u" + strconv.Itoa(i)
		ts := epoch.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		treat := rng.Float64() >= o.Split
		variant, rate, mean := "control", o.BaseRate, o.MeanRevenue
		if treat {
			variant, rate, mean = "treatment", o.BaseRate+o.Lift, o.MeanRevenue*(1+o.RevenueLift)
		}
		conv, rev := "0", "0"
		if rng.Float64() < rate {
			conv = "1"
			rev = strconv.FormatFloat(rng.ExpFloat64()*mean, 'f', 2, 64)
		}
		if err := emit(id, user, variant, ts, conv, rev); err != nil {
			return err
		}
		switch r := rng.Float64(); {
		case r < o.DupRate:
			if _, err := bw.WriteString(prev); err != nil {
				return err
			}
		case r < o.DupRate+o.CrossRate:
			other := "treatment"
			if treat {
				other = "control"
			}
			if err := emit(id+"x", user, other, ts, conv, rev); err != nil {
				return err
			}
		case r < o.DupRate+o.CrossRate+o.BadRate:
			bad := corrupt(rng.IntN(4), o.Format)
			if _, err := bw.WriteString(bad); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func corrupt(kind int, format string) string {
	if format == "csv" {
		switch kind {
		case 0:
			return "bad,ub,banana,2026-10-01T00:00:00Z,1,5\n"
		case 1:
			return "badc,uc,control,not-a-time,1,5\n"
		case 2:
			return "badd,ud,control,2026-10-01T00:00:00Z,maybe,5\n"
		}
		return "bade,ue,control,2026-10-01T00:00:00Z,1,-5\n"
	}
	switch kind {
	case 0:
		return "{\"event_id\":\"bad\",\"user_id\":\"ub\",\"variant\":\"control\",\"timestamp\":\"2026-10-01T00:00:00Z\",\"converted\":1,\"revenue\":\n"
	case 1:
		return "{\"event_id\":\"badc\",\"user_id\":\"uc\",\"variant\":\"control\",\"timestamp\":\"yesterday\",\"converted\":1}\n"
	case 2:
		return "{\"event_id\":\"badd\",\"user_id\":\"ud\",\"variant\":\"control\",\"timestamp\":\"2026-10-01T00:00:00Z\"}\n"
	}
	return "{\"event_id\":\"bade\",\"user_id\":\"ue\",\"variant\":\"control\",\"timestamp\":\"2026-10-01T00:00:00Z\",\"converted\":1,\"revenue\":-5}\n"
}
