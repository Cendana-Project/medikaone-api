package response

import (
	"cmp"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"
)

// ScheduleGroup contains the original schedule rows or proposal items alongside
// their display labels. Only active rows may be used for booking/deletion.
// Parent response Schedules fields stay internal; rows are serialized here only.
type ScheduleGroup struct {
	Type                string                   `json:"type"`
	Status              string                   `json:"status,omitempty"`
	ItemIDs             []string                 `json:"item_ids"`
	Schedules           []DoctorHospitalSchedule `json:"schedules"`
	DayOfWeek           []int                    `json:"day_of_week"`
	ScheduleDate        *string                  `json:"schedule_date,omitempty"`
	DayLabel            string                   `json:"day_label"`
	TimeLabel           string                   `json:"time_label"`
	DisplayLabel        string                   `json:"display_label"`
	StartTime           string                   `json:"start_time"`
	EndTime             string                   `json:"end_time"`
	Timezone            string                   `json:"timezone"`
	BookingMode         string                   `json:"booking_mode"`
	SlotDurationMinutes int                      `json:"slot_duration_minutes"`
	Capacity            int                      `json:"capacity"`
}

// GroupSchedules combines recurring days only when the entire practice window
// and booking configuration match. Adjacent sessions retain their own boundaries.
// A caller's collection already scopes doctor/hospital/affiliation/proposal, so
// groups can never join schedules belonging to different resources.
func GroupSchedules(schedules []DoctorHospitalSchedule, fallbackStatus string) []ScheduleGroup {
	type groupKey struct {
		status, start, end, timezone, mode string
		duration, capacity                 int
	}
	groups := make([]ScheduleGroup, 0)
	indices := make(map[groupKey]int)
	for _, schedule := range schedules {
		status := schedule.Status
		if status == "" {
			status = fallbackStatus
		}
		key := groupKey{status, schedule.StartTime, schedule.EndTime, schedule.Timezone,
			schedule.BookingMode, schedule.SlotDurationMinutes, schedule.Capacity}
		index, exists := indices[key]
		if !exists || schedule.ScheduleDate != nil {
			index = len(groups)
			group := ScheduleGroup{
				Type: "RECURRING", Status: status, ItemIDs: []string{}, DayOfWeek: []int{},
				Schedules:    []DoctorHospitalSchedule{},
				ScheduleDate: schedule.ScheduleDate, StartTime: schedule.StartTime,
				EndTime: schedule.EndTime, Timezone: schedule.Timezone,
				BookingMode: schedule.BookingMode, SlotDurationMinutes: schedule.SlotDurationMinutes,
				Capacity: schedule.Capacity,
			}
			if schedule.ScheduleDate != nil {
				group.Type = "SPECIFIC"
			} else {
				indices[key] = index
			}
			groups = append(groups, group)
		}
		group := &groups[index]
		// Copy before applying a display fallback; never mutate the source rows.
		schedule.Status = status
		group.Schedules = append(group.Schedules, schedule)
		if schedule.ID != "" {
			group.ItemIDs = append(group.ItemIDs, schedule.ID)
		}
		if schedule.ScheduleDate == nil && schedule.DayOfWeek >= 0 && schedule.DayOfWeek <= 6 {
			group.DayOfWeek = append(group.DayOfWeek, schedule.DayOfWeek)
		}
	}
	for i := range groups {
		group := &groups[i]
		sort.Strings(group.ItemIDs)
		sort.Slice(group.Schedules, func(i, j int) bool {
			left, right := group.Schedules[i], group.Schedules[j]
			return cmp.Or(
				cmp.Compare(weekdayOrder(left.DayOfWeek), weekdayOrder(right.DayOfWeek)),
				cmp.Compare(left.ID, right.ID),
			) < 0
		})
		sort.Slice(group.DayOfWeek, func(i, j int) bool { return weekdayOrder(group.DayOfWeek[i]) < weekdayOrder(group.DayOfWeek[j]) })
		days := make([]int, 0, len(group.DayOfWeek))
		for _, day := range group.DayOfWeek {
			if len(days) == 0 || days[len(days)-1] != day {
				days = append(days, day)
			}
		}
		group.DayOfWeek = days
		group.DayLabel = weekdayLabel(days)
		if group.ScheduleDate != nil {
			group.DayLabel = dateLabel(*group.ScheduleDate)
		}
		group.TimeLabel = group.StartTime + "–" + group.EndTime
		group.DisplayLabel = group.DayLabel + ", " + group.TimeLabel
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.Type != b.Type {
			return a.Type == "RECURRING"
		}
		if a.ScheduleDate != nil && b.ScheduleDate != nil && *a.ScheduleDate != *b.ScheduleDate {
			return *a.ScheduleDate < *b.ScheduleDate
		}
		if len(a.DayOfWeek) > 0 && len(b.DayOfWeek) > 0 && a.DayOfWeek[0] != b.DayOfWeek[0] {
			return weekdayOrder(a.DayOfWeek[0]) < weekdayOrder(b.DayOfWeek[0])
		}
		if a.StartTime != b.StartTime {
			return a.StartTime < b.StartTime
		}
		return cmp.Or(
			cmp.Compare(a.EndTime, b.EndTime),
			cmp.Compare(a.Timezone, b.Timezone),
			cmp.Compare(a.Status, b.Status),
			cmp.Compare(a.BookingMode, b.BookingMode),
			cmp.Compare(a.SlotDurationMinutes, b.SlotDurationMinutes),
			cmp.Compare(a.Capacity, b.Capacity),
			slices.Compare(a.DayOfWeek, b.DayOfWeek),
			slices.Compare(a.ItemIDs, b.ItemIDs),
		) < 0
	})
	return groups
}

var weekdayNames = [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

func weekdayOrder(day int) int { return (day + 6) % 7 }

func weekdayLabel(days []int) string {
	parts := []string{}
	for start := 0; start < len(days); {
		end := start
		for end+1 < len(days) && weekdayOrder(days[end+1]) == weekdayOrder(days[end])+1 {
			end++
		}
		label := weekdayNames[days[start]]
		if end > start {
			label += "–" + weekdayNames[days[end]]
		}
		parts = append(parts, label)
		start = end + 1
	}
	return strings.Join(parts, ", ")
}

func dateLabel(value string) string {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	months := [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
	return date.Format("02") + " " + months[int(date.Month())-1] + " " + date.Format("2006")
}

func (v DoctorHospitalInvitation) MarshalJSON() ([]byte, error) {
	type fields DoctorHospitalInvitation
	return json.Marshal(struct {
		fields
		ScheduleGroups []ScheduleGroup `json:"schedule_groups"`
	}{fields(v), GroupSchedules(v.Schedules, "")})
}

func (v HospitalDoctor) MarshalJSON() ([]byte, error) {
	type fields HospitalDoctor
	return json.Marshal(struct {
		fields
		ScheduleGroups []ScheduleGroup `json:"schedule_groups"`
	}{fields(v), GroupSchedules(v.Schedules, "ACTIVE")})
}

// Override the embedded HospitalDoctor marshaler so affiliation detail keeps its
// hospital and originating invitation in addition to the grouped schedules.
func (v DoctorHospitalAffiliationDetail) MarshalJSON() ([]byte, error) {
	type fields HospitalDoctor
	return json.Marshal(struct {
		fields
		Hospital       *HospitalInformation  `json:"hospital"`
		Invitation     AffiliationInvitation `json:"invitation"`
		ScheduleGroups []ScheduleGroup       `json:"schedule_groups"`
	}{fields(v.HospitalDoctor), v.Hospital, v.Invitation, GroupSchedules(v.Schedules, "ACTIVE")})
}

func (v PendingScheduleChange) MarshalJSON() ([]byte, error) {
	type fields PendingScheduleChange
	return json.Marshal(struct {
		fields
		ScheduleGroups []ScheduleGroup `json:"schedule_groups"`
	}{fields(v), GroupSchedules(v.Schedules, v.Status)})
}

func (v ScheduleChangeRequest) MarshalJSON() ([]byte, error) {
	type fields ScheduleChangeRequest
	return json.Marshal(struct {
		fields
		ScheduleGroups []ScheduleGroup `json:"schedule_groups"`
	}{fields(v), GroupSchedules(v.Schedules, v.Status)})
}

func (v PublicDoctorAffiliation) MarshalJSON() ([]byte, error) {
	type fields PublicDoctorAffiliation
	return json.Marshal(struct {
		fields
		ScheduleGroups []ScheduleGroup `json:"schedule_groups"`
	}{fields(v), GroupSchedules(v.Schedules, "ACTIVE")})
}
