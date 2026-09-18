package util

import (
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

// ScheduleDays expands a recurring schedule or derives the weekday of a one-off date.
func ScheduleDays(days []int, date *string) ([]int, error) {
	if date != nil {
		parsed, err := time.Parse("2006-01-02", *date)
		if err != nil || parsed.Year() < 1 || parsed.Format("2006-01-02") != *date || len(days) != 0 {
			return nil, constant.NewInvalidFieldValueError("schedule_date", "a YYYY-MM-DD date with an empty day_of_week array", "tanggal YYYY-MM-DD dengan array day_of_week kosong")
		}
		return []int{int(parsed.Weekday())}, nil
	}
	if len(days) == 0 || len(days) > 7 {
		return nil, constant.NewInvalidFieldValueError("day_of_week", "an array of 1 to 7 distinct integers from 0 through 6", "array berisi 1 sampai 7 angka unik dari 0 sampai 6")
	}
	seen := map[int]bool{}
	for _, day := range days {
		if day < 0 || day > 6 || seen[day] {
			return nil, constant.NewInvalidFieldValueError("day_of_week", "an array of distinct integers from 0 through 6", "array angka unik dari 0 sampai 6")
		}
		seen[day] = true
	}
	return days, nil
}

func ScheduleDatesOverlap(dayA int, dateA *string, dayB int, dateB *string) bool {
	return dayA == dayB && (dateA == nil || dateB == nil || *dateA == *dateB)
}

func ScheduleMatchesDate(day int, specificDate *string, date time.Time) bool {
	if specificDate != nil {
		return *specificDate == date.Format("2006-01-02")
	}
	return day == int(date.Weekday())
}

func ScheduleWeekdays(day int, date *string) []int {
	if date != nil {
		return []int{}
	}
	return []int{day}
}
