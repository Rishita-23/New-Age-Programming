package validate

import (
	"context"
	"errors"
	"math"
	"strings"

	"ablens/internal/config"
	"ablens/internal/model"
	"ablens/internal/result"
	"ablens/internal/stats"
)

type compact struct {
	uid int32
	obs model.Obs
}

func Run(ctx context.Context, in <-chan []model.Row, cfg config.Config) ([]model.Obs, result.Quality, error) {
	q := result.Quality{MalformedReasons: map[string]int{}}
	seenEvents := map[string]struct{}{}
	userIdx := map[string]int32{}
	var userVariant []uint8
	var conflicted []bool
	var kept []compact

	for {
		select {
		case <-ctx.Done():
			return nil, q, ctx.Err()
		case batch, ok := <-in:
			if !ok {
				return finish(kept, conflicted, q, cfg)
			}
			for _, row := range batch {
				q.TotalRows++
				if row.Err != nil {
					q.MalformedRows++
					q.MalformedReasons[row.Err.Error()]++
					continue
				}
				e := row.Event
				if reason := semantic(e); reason != "" {
					q.MalformedRows++
					q.MalformedReasons[reason]++
					continue
				}
				var v uint8
				switch e.Variant {
				case cfg.Control:
					v = 0
				case cfg.Treatment:
					v = 1
				default:
					q.UnknownVariant++
					continue
				}
				if _, dup := seenEvents[e.EventID]; dup {
					q.DuplicateEvents++
					continue
				}
				seenEvents[strings.Clone(e.EventID)] = struct{}{}
				uid, known := userIdx[e.UserID]
				if !known {
					uid = int32(len(userVariant))
					userIdx[strings.Clone(e.UserID)] = uid
					userVariant = append(userVariant, v)
					conflicted = append(conflicted, false)
				} else if userVariant[uid] != v && !conflicted[uid] {
					conflicted[uid] = true
					q.CrossVariantUsers++
				}
				kept = append(kept, compact{uid: uid, obs: model.Obs{Variant: v, Converted: uint8(e.Converted), Revenue: e.Revenue}})
			}
		}
	}
}

func semantic(e model.Event) string {
	switch {
	case e.EventID == "":
		return "missing event_id"
	case e.UserID == "":
		return "missing user_id"
	case e.Variant == "":
		return "missing variant"
	case e.Converted != 0 && e.Converted != 1:
		return "invalid converted"
	case math.IsNaN(e.Revenue) || math.IsInf(e.Revenue, 0) || e.Revenue < 0:
		return "invalid revenue"
	}
	return ""
}

func finish(kept []compact, conflicted []bool, q result.Quality, cfg config.Config) ([]model.Obs, result.Quality, error) {
	clean := make([]model.Obs, 0, len(kept))
	for _, k := range kept {
		if conflicted[k.uid] {
			q.CrossVariantRows++
			continue
		}
		clean = append(clean, k.obs)
	}
	q.ValidRows = len(clean)
	if len(q.MalformedReasons) == 0 {
		q.MalformedReasons = nil
	}
	if len(clean) == 0 {
		return nil, q, errors.New("no valid rows after validation")
	}
	for _, o := range clean {
		q.SRM.Observed[o.Variant]++
	}
	total := float64(len(clean))
	q.SRM.Expected = [2]float64{cfg.ExpectedSplit[0] * total, cfg.ExpectedSplit[1] * total}
	chi, _, p, err := stats.ChiSquareGOF(q.SRM.Observed[:], q.SRM.Expected[:])
	if err != nil {
		return nil, q, err
	}
	q.SRM.ChiSquare, q.SRM.PValue = chi, p
	q.SRM.Flagged = p < cfg.SRMAlpha
	return clean, q, nil
}
