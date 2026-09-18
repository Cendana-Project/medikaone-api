package util

import (
	"reflect"
	"testing"
	"time"
)

func TestScheduleDaysValidatesRecurrenceAndSpecificDate(t *testing.T) {
	date, invalid, yearZero := "2026-09-14", "2026-02-30", "0000-01-01"
	for _, test := range []struct {
		name  string
		days  []int
		date  *string
		want  []int
		valid bool
	}{
		{"multiple weekdays", []int{0, 2, 6}, nil, []int{0, 2, 6}, true},
		{"one off", nil, &date, []int{1}, true},
		{"no recurrence", nil, nil, nil, false},
		{"duplicate weekdays", []int{1, 1}, nil, nil, false},
		{"invalid weekday", []int{7}, nil, nil, false},
		{"negative weekday", []int{-1}, nil, nil, false},
		{"invalid date", nil, &invalid, nil, false},
		{"unsupported year zero", nil, &yearZero, nil, false},
		{"both recurrence kinds", []int{1}, &date, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ScheduleDays(test.days, test.date)
			if (err == nil) != test.valid || (test.valid && !reflect.DeepEqual(got, test.want)) {
				t.Fatalf("days=%v err=%v", got, err)
			}
		})
	}
}

func TestScheduleOverlapDistinguishesSpecificDates(t *testing.T) {
	a, b := "2026-09-14", "2026-09-21"
	if ScheduleDatesOverlap(1, &a, 1, &b) {
		t.Fatal("separate Mondays must not conflict")
	}
	if !ScheduleDatesOverlap(1, &a, 1, nil) || !ScheduleDatesOverlap(1, nil, 1, &a) || !ScheduleDatesOverlap(1, &a, 1, &a) {
		t.Fatal("same occurrence must conflict")
	}
	date, _ := time.Parse("2006-01-02", b)
	if ScheduleMatchesDate(1, &a, date) {
		t.Fatal("one-off repeated on following Monday")
	}
	if !ScheduleMatchesDate(1, nil, date) {
		t.Fatal("recurring Monday must match")
	}
}
