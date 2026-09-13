package response

import (
	"encoding/json"
)

// Each stored recurrence keeps its own ID; weekdays are arrays at the API boundary.
func (s DoctorHospitalSchedule) MarshalJSON() ([]byte, error) {
	type scheduleAlias DoctorHospitalSchedule
	days := []int{s.DayOfWeek}
	if s.ScheduleDate != nil {
		days = []int{}
	}
	return json.Marshal(struct {
		scheduleAlias
		DayOfWeek []int `json:"day_of_week"`
	}{scheduleAlias(s), days})
}
