package directorycriteria

import (
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

// Availability times refer to the local timezone of each matching schedule.
type Availability struct {
	Date, From, To, BookingMode string
	OnlyAvailable               bool
}

func (q *Availability) Validate(now time.Time) error {
	q.Date, q.From, q.To = strings.TrimSpace(q.Date), strings.TrimSpace(q.From), strings.TrimSpace(q.To)
	q.BookingMode = strings.ToUpper(strings.TrimSpace(q.BookingMode))
	if q.Date != "" {
		date, err := time.Parse("2006-01-02", q.Date)
		today := now.UTC().Truncate(24 * time.Hour)
		if err != nil || date.Before(today) || date.After(today.AddDate(1, 0, 0)) {
			return constant.NewInvalidFieldValueError("available_on", "a date from today through one year ahead in YYYY-MM-DD format", "tanggal hari ini hingga satu tahun ke depan dengan format YYYY-MM-DD")
		}
	}
	if (q.From != "" || q.To != "" || q.OnlyAvailable) && q.Date == "" {
		return constant.NewFieldRequiredError("available_on")
	}
	if (q.From == "") != (q.To == "") {
		return constant.NewInvalidFieldValueError("available_from/available_to", "both times supplied together", "kedua jam dikirim berpasangan")
	}
	if q.From != "" {
		from, e1 := time.Parse("15:04", q.From)
		to, e2 := time.Parse("15:04", q.To)
		if len(q.From) != 5 || len(q.To) != 5 || e1 != nil || e2 != nil || !to.After(from) {
			return constant.NewInvalidFieldValueError("available_from/available_to", "HH:mm with end after start on the same day", "HH:mm dengan jam akhir setelah jam awal pada hari yang sama")
		}
	}
	if q.BookingMode != "" && q.BookingMode != "FIXED_SLOT" && q.BookingMode != "SESSION_QUEUE" {
		return constant.NewInvalidFieldValueError("booking_mode", "FIXED_SLOT or SESSION_QUEUE", "FIXED_SLOT atau SESSION_QUEUE")
	}
	return nil
}

func PracticeStartedOn(raw *string, now time.Time) error {
	if raw == nil {
		return nil
	}
	date, err := time.Parse("2006-01-02", *raw)
	if err != nil || len(*raw) != 10 || date.Year() < 1900 || date.After(now.UTC().Truncate(24*time.Hour)) {
		return constant.NewInvalidFieldValueError("practice_started_on", "YYYY-MM-DD from 1900-01-01 through today", "YYYY-MM-DD dari 1900-01-01 sampai hari ini")
	}
	return nil
}
