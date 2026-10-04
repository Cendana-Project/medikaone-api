package directorycriteria

import (
	"testing"
	"time"
)

func TestAvailabilityValidation(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	for _, q := range []Availability{
		{OnlyAvailable: true}, {From: "09:00", To: "10:00"},
		{Date: "2026-10-05", From: "09:00"}, {Date: "2026-10-05", To: "10:00"},
		{Date: "2026-10-05", From: "9:00", To: "10:00"},
		{Date: "2026-10-05", From: "09:00", To: "09:00"},
		{Date: "2026-10-05", From: "23:00", To: "01:00"},
		{Date: "2026-10-03"}, {Date: "2026-02-30"}, {Date: "2027-10-05"},
		{BookingMode: "INVALID"},
	} {
		if err := q.Validate(now); err == nil {
			t.Errorf("accepted invalid filter: %#v", q)
		}
	}
	for _, q := range []Availability{{}, {Date: "2026-10-04"}, {Date: "2026-10-05", From: "09:00", To: "10:00", OnlyAvailable: true, BookingMode: "session_queue"}} {
		if err := q.Validate(now); err != nil {
			t.Errorf("valid filter: %#v %v", q, err)
		}
	}
}

func TestPracticeStartDateValidation(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	for _, value := range []string{"", "2026-10-05", "2026-02-30", "1899-12-31", "2020-1-01", " 2020-01-01 "} {
		if err := PracticeStartedOn(&value, now); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	for _, value := range []string{"2026-10-04", "2000-02-29", "1900-01-01"} {
		if err := PracticeStartedOn(&value, now); err != nil {
			t.Error(err)
		}
	}
	if err := PracticeStartedOn(nil, now); err != nil {
		t.Error(err)
	}
}
