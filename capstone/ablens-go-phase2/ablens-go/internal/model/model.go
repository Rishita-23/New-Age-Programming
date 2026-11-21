package model

import "time"

type Event struct {
	EventID   string
	UserID    string
	Variant   string
	Timestamp time.Time
	Converted int
	Revenue   float64
}

type Row struct {
	Line  int
	Event Event
	Err   error
}

type Obs struct {
	Variant   uint8
	Converted uint8
	Revenue   float64
}
