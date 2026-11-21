package validate

import (
	"context"
	"errors"
	"testing"

	"ablens/internal/config"
	"ablens/internal/model"
)

func feed(rows ...model.Row) <-chan []model.Row {
	ch := make(chan []model.Row, 1)
	ch <- rows
	close(ch)
	return ch
}

func ev(id, user, variant string, conv int, rev float64) model.Row {
	return model.Row{Event: model.Event{EventID: id, UserID: user, Variant: variant, Converted: conv, Revenue: rev}}
}

func TestQualityChecks(t *testing.T) {
	cfg := config.Default()
	rows := []model.Row{
		ev("e1", "u1", "control", 1, 5),
		ev("e2", "u2", "treatment", 0, 0),
		ev("e1", "u1", "control", 1, 5),
		ev("e3", "u3", "control", 0, 0),
		ev("e4", "u3", "treatment", 1, 1),
		ev("e5", "u5", "control", 2, 0),
		ev("e6", "u6", "control", 1, -1),
		ev("", "u7", "control", 1, 1),
		ev("e8", "u8", "banana", 1, 1),
		{Line: 99, Err: errors.New("invalid timestamp")},
	}
	clean, q, err := Run(context.Background(), feed(rows...), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if q.TotalRows != 10 || q.DuplicateEvents != 1 || q.CrossVariantUsers != 1 || q.CrossVariantRows != 2 ||
		q.UnknownVariant != 1 || q.MalformedRows != 4 {
		t.Errorf("unexpected quality: %+v", q)
	}
	for _, reason := range []string{"invalid converted", "invalid revenue", "missing event_id", "invalid timestamp"} {
		if q.MalformedReasons[reason] == 0 {
			t.Errorf("missing malformed reason %q in %v", reason, q.MalformedReasons)
		}
	}
	if q.ValidRows != 2 || len(clean) != 2 {
		t.Errorf("valid=%d clean=%d, want 2 (u1 control, u2 treatment)", q.ValidRows, len(clean))
	}
	if q.SRM.Observed != [2]int{1, 1} {
		t.Errorf("SRM observed %v", q.SRM.Observed)
	}
}

func TestCrossVariantUserCountedOnceAndAllRowsDropped(t *testing.T) {
	cfg := config.Default()
	rows := []model.Row{
		ev("e1", "u1", "control", 0, 0),
		ev("e2", "u1", "treatment", 0, 0),
		ev("e3", "u1", "control", 0, 0),
		ev("e4", "u2", "control", 0, 0),
		ev("e5", "u3", "treatment", 0, 0),
	}
	_, q, err := Run(context.Background(), feed(rows...), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if q.CrossVariantUsers != 1 || q.CrossVariantRows != 3 || q.ValidRows != 2 {
		t.Errorf("unexpected quality: %+v", q)
	}
}

func TestSRMFlagging(t *testing.T) {
	cfg := config.Default()
	var rows []model.Row
	for i := 0; i < 600; i++ {
		rows = append(rows, ev("c"+string(rune('a'+i%26))+itoa(i), "uc"+itoa(i), "control", 0, 0))
	}
	for i := 0; i < 400; i++ {
		rows = append(rows, ev("t"+itoa(i), "ut"+itoa(i), "treatment", 0, 0))
	}
	_, q, err := Run(context.Background(), feed(rows...), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !q.SRM.Flagged || q.SRM.Observed != [2]int{600, 400} {
		t.Errorf("expected SRM flag for 600/400: %+v", q.SRM)
	}
	if q.SRM.ChiSquare != 40 {
		t.Errorf("chi-square: got %v want 40", q.SRM.ChiSquare)
	}

	rows = rows[:0]
	for i := 0; i < 500; i++ {
		rows = append(rows, ev("c"+itoa(i), "uc"+itoa(i), "control", 0, 0), ev("t"+itoa(i), "ut"+itoa(i), "treatment", 0, 0))
	}
	_, q, _ = Run(context.Background(), feed(rows...), cfg)
	if q.SRM.Flagged {
		t.Errorf("balanced data flagged: %+v", q.SRM)
	}
}

func TestNoValidRows(t *testing.T) {
	_, q, err := Run(context.Background(), feed(model.Row{Err: errors.New("invalid json")}), config.Default())
	if err == nil {
		t.Fatal("expected error")
	}
	if q.MalformedRows != 1 {
		t.Errorf("quality: %+v", q)
	}
}

func TestContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan []model.Row)
	if _, _, err := Run(ctx, ch, config.Default()); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
