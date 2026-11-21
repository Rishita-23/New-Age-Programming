package ingest

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ablens/internal/model"
)

const batchSize = 2048

type Reader interface {
	Read(ctx context.Context, r io.Reader, out chan<- []model.Row) error
}

func ForPath(path string) (Reader, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return CSV{}, nil
	case ".jsonl", ".json", ".ndjson":
		return JSONL{}, nil
	}
	return nil, fmt.Errorf("unsupported input format for %q (use .csv or .jsonl)", path)
}

func ReadFile(ctx context.Context, path string, out chan<- []model.Row) error {
	rd, err := ForPath(path)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return rd.Read(ctx, f, out)
}

func flush(ctx context.Context, out chan<- []model.Row, batch []model.Row) error {
	select {
	case out <- batch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type CSV struct{}

type columns struct {
	event, user, variant, ts, conv, rev int
	width                               int
}

func findColumns(header []string) (columns, error) {
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	c := columns{rev: -1}
	var missing []string
	get := func(name string, dst *int) {
		i, ok := idx[name]
		if !ok {
			missing = append(missing, name)
			return
		}
		*dst = i
		if i+1 > c.width {
			c.width = i + 1
		}
	}
	get("event_id", &c.event)
	get("user_id", &c.user)
	get("variant", &c.variant)
	get("timestamp", &c.ts)
	get("converted", &c.conv)
	if i, ok := idx["revenue"]; ok {
		c.rev = i
		if i+1 > c.width {
			c.width = i + 1
		}
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("csv header is missing required column(s): %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func (CSV) Read(ctx context.Context, r io.Reader, out chan<- []model.Row) error {
	cr := csv.NewReader(bufio.NewReaderSize(r, 1<<20))
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true
	header, err := cr.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("csv input is empty")
		}
		return fmt.Errorf("read csv header: %w", err)
	}
	cols, err := findColumns(header)
	if err != nil {
		return err
	}
	batch := make([]model.Row, 0, batchSize)
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		var pe *csv.ParseError
		if err != nil && !errors.As(err, &pe) {
			return fmt.Errorf("read csv: %w", err)
		}
		line := 0
		if pe != nil {
			line = pe.Line
			batch = append(batch, model.Row{Line: line, Err: errors.New("malformed csv line")})
		} else {
			line, _ = cr.FieldPos(0)
			batch = append(batch, csvRow(rec, cols, line))
		}
		if len(batch) == batchSize {
			if err := flush(ctx, out, batch); err != nil {
				return err
			}
			batch = make([]model.Row, 0, batchSize)
		}
	}
	if len(batch) > 0 {
		return flush(ctx, out, batch)
	}
	return nil
}

func csvRow(rec []string, c columns, line int) model.Row {
	if len(rec) < c.width {
		return model.Row{Line: line, Err: errors.New("wrong field count")}
	}
	ev := model.Event{EventID: rec[c.event], UserID: rec[c.user], Variant: rec[c.variant]}
	ts, err := time.Parse(time.RFC3339, rec[c.ts])
	if err != nil {
		return model.Row{Line: line, Err: errors.New("invalid timestamp")}
	}
	ev.Timestamp = ts
	conv, err := strconv.Atoi(rec[c.conv])
	if err != nil {
		return model.Row{Line: line, Err: errors.New("invalid converted")}
	}
	ev.Converted = conv
	if c.rev >= 0 && rec[c.rev] != "" {
		rev, err := strconv.ParseFloat(rec[c.rev], 64)
		if err != nil {
			return model.Row{Line: line, Err: errors.New("invalid revenue")}
		}
		ev.Revenue = rev
	}
	return model.Row{Line: line, Event: ev}
}

type JSONL struct{}

type jsonEvent struct {
	EventID   string   `json:"event_id"`
	UserID    string   `json:"user_id"`
	Variant   string   `json:"variant"`
	Timestamp string   `json:"timestamp"`
	Converted *int     `json:"converted"`
	Revenue   *float64 `json:"revenue"`
}

func (JSONL) Read(ctx context.Context, r io.Reader, out chan<- []model.Row) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	batch := make([]model.Row, 0, batchSize)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		batch = append(batch, jsonRow(text, line))
		if len(batch) == batchSize {
			if err := flush(ctx, out, batch); err != nil {
				return err
			}
			batch = make([]model.Row, 0, batchSize)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read jsonl: %w", err)
	}
	if len(batch) > 0 {
		return flush(ctx, out, batch)
	}
	return nil
}

func jsonRow(text string, line int) model.Row {
	var je jsonEvent
	if err := json.Unmarshal([]byte(text), &je); err != nil {
		return model.Row{Line: line, Err: errors.New("invalid json")}
	}
	ts, err := time.Parse(time.RFC3339, je.Timestamp)
	if err != nil {
		return model.Row{Line: line, Err: errors.New("invalid timestamp")}
	}
	if je.Converted == nil {
		return model.Row{Line: line, Err: errors.New("invalid converted")}
	}
	ev := model.Event{EventID: je.EventID, UserID: je.UserID, Variant: je.Variant, Timestamp: ts, Converted: *je.Converted}
	if je.Revenue != nil {
		ev.Revenue = *je.Revenue
	}
	return model.Row{Line: line, Event: ev}
}
