package ingest

import (
	"context"
	"strings"
	"testing"

	"ablens/internal/model"
)

func collect(t *testing.T, rd Reader, src string) ([]model.Row, error) {
	t.Helper()
	out := make(chan []model.Row, 4)
	var rows []model.Row
	done := make(chan struct{})
	go func() {
		for b := range out {
			rows = append(rows, b...)
		}
		close(done)
	}()
	err := rd.Read(context.Background(), strings.NewReader(src), out)
	close(out)
	<-done
	return rows, err
}

func TestCSVParsesRowsAndFlagsBadOnes(t *testing.T) {
	src := "event_id,user_id,variant,timestamp,converted,revenue\n" +
		"e1,u1,control,2026-10-01T00:00:00Z,1,12.5\n" +
		"e2,u2,treatment,2026-10-01T00:00:01Z,0,\n" +
		"e3,u3,control,yesterday,1,5\n" +
		"e4,u4,control,2026-10-01T00:00:00Z,yes,5\n" +
		"e5,u5,control,2026-10-01T00:00:00Z,1,abc\n" +
		"e6,u6,control\n"
	rows, err := collect(t, CSV{}, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 6 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].Err != nil || rows[0].Event.Revenue != 12.5 || rows[0].Event.Converted != 1 || rows[0].Event.Variant != "control" {
		t.Errorf("row0: %+v", rows[0])
	}
	if rows[1].Err != nil || rows[1].Event.Revenue != 0 {
		t.Errorf("row1: %+v", rows[1])
	}
	want := []string{"invalid timestamp", "invalid converted", "invalid revenue", "wrong field count"}
	for i, w := range want {
		if rows[i+2].Err == nil || rows[i+2].Err.Error() != w {
			t.Errorf("row %d: got %v want %q", i+2, rows[i+2].Err, w)
		}
	}
	if rows[2].Line != 4 {
		t.Errorf("line number: got %d want 4", rows[2].Line)
	}
}

func TestCSVHeaderHandling(t *testing.T) {
	src := "\ufeffUser_ID, Event_ID ,variant,timestamp,converted\nu1,e1,control,2026-10-01T00:00:00Z,1\n"
	rows, err := collect(t, CSV{}, src)
	if err != nil || len(rows) != 1 || rows[0].Err != nil || rows[0].Event.EventID != "e1" || rows[0].Event.UserID != "u1" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if _, err := collect(t, CSV{}, "event_id,user_id\ne1,u1\n"); err == nil || !strings.Contains(err.Error(), "variant") {
		t.Errorf("expected missing-column error, got %v", err)
	}
	if _, err := collect(t, CSV{}, ""); err == nil {
		t.Error("expected error for empty input")
	}
}

func TestCSVMalformedQuoteDoesNotAbort(t *testing.T) {
	src := "event_id,user_id,variant,timestamp,converted\n" +
		"e1,u1,control,2026-10-01T00:00:00Z,1\n" +
		"e2,\"u2,control,2026-10-01T00:00:00Z,1\n" +
		"e3,u3,control,2026-10-01T00:00:00Z,0\n"
	rows, err := collect(t, CSV{}, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 || rows[0].Err != nil {
		t.Fatalf("rows=%+v", rows)
	}
	bad := 0
	for _, r := range rows {
		if r.Err != nil {
			bad++
		}
	}
	if bad == 0 {
		t.Error("expected at least one flagged row")
	}
}

func TestJSONL(t *testing.T) {
	src := `{"event_id":"e1","user_id":"u1","variant":"control","timestamp":"2026-10-01T00:00:00Z","converted":1,"revenue":9.5}` + "\n\n" +
		`{"event_id":"e2","user_id":"u2","variant":"treatment","timestamp":"2026-10-01T00:00:00Z","converted":0}` + "\n" +
		`{"event_id":"e3","user_id":"u3","variant":"control","timestamp":"2026-10-01T00:00:00Z"}` + "\n" +
		`{"event_id":"e4",` + "\n" +
		`{"event_id":"e5","user_id":"u5","variant":"control","timestamp":"nope","converted":1}` + "\n"
	rows, err := collect(t, JSONL{}, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("got %d rows (blank line should be skipped)", len(rows))
	}
	if rows[0].Err != nil || rows[0].Event.Revenue != 9.5 {
		t.Errorf("row0: %+v", rows[0])
	}
	if rows[1].Err != nil || rows[1].Event.Revenue != 0 {
		t.Errorf("row1: %+v", rows[1])
	}
	want := []string{"invalid converted", "invalid json", "invalid timestamp"}
	for i, w := range want {
		if rows[i+2].Err == nil || rows[i+2].Err.Error() != w {
			t.Errorf("row %d: got %v want %q", i+2, rows[i+2].Err, w)
		}
	}
	if rows[2].Line != 4 {
		t.Errorf("line number counts blank lines: got %d want 4", rows[2].Line)
	}
}

func TestForPath(t *testing.T) {
	for _, p := range []string{"a.csv", "a.CSV", "a.jsonl", "a.json", "a.ndjson"} {
		if _, err := ForPath(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, err := ForPath("a.xlsx"); err == nil {
		t.Error("expected unsupported format error")
	}
}

func TestReadCancelled(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("event_id,user_id,variant,timestamp,converted\n")
	for i := 0; i < 10000; i++ {
		sb.WriteString("e,u,control,2026-10-01T00:00:00Z,1\n")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := make(chan []model.Row)
	if err := (CSV{}).Read(ctx, strings.NewReader(sb.String()), out); err == nil {
		t.Error("expected context error")
	}
}
