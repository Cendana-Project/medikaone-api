package response

import (
	"encoding/json"
	"reflect"
	"testing"
)

func recurringDisplayFixture() []DoctorHospitalSchedule {
	rows := make([]DoctorHospitalSchedule, 0, 4)
	for _, day := range []int{4, 2, 1, 3} {
		rows = append(rows, DoctorHospitalSchedule{
			ID: weekdayNames[day], DayOfWeek: day, StartTime: "19:00", EndTime: "20:00",
			Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 1,
		})
	}
	return rows
}

func TestScheduleGroupsCompactWeekdaysAndPreserveSourceRows(t *testing.T) {
	rows := recurringDisplayFixture()
	before := append([]DoctorHospitalSchedule(nil), rows...)
	groups := GroupSchedules(rows, "ACTIVE")
	if len(groups) != 1 || groups[0].DisplayLabel != "Senin–Kamis, 19:00–20:00" ||
		groups[0].Status != "ACTIVE" || groups[0].Type != "RECURRING" || len(groups[0].ItemIDs) != 4 ||
		len(groups[0].Schedules) != 4 ||
		!reflect.DeepEqual(groups[0].DayOfWeek, []int{1, 2, 3, 4}) {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	if !reflect.DeepEqual(before, rows) {
		t.Fatal("display projection mutated the original schedule rows")
	}
	for i, schedule := range groups[0].Schedules {
		if schedule.DayOfWeek != i+1 || schedule.Status != "ACTIVE" || schedule.ID != weekdayNames[i+1] {
			t.Fatalf("nested row lost identity/status or weekday order: %+v", schedule)
		}
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	if shuffled := GroupSchedules(rows, "ACTIVE"); !reflect.DeepEqual(groups, shuffled) {
		t.Fatalf("grouping depends on query order: %+v", shuffled)
	}
}

func TestScheduleGroupsDoNotCombineDifferentSessionsOrBookingRules(t *testing.T) {
	base := recurringDisplayFixture()[0]
	variants := []func(*DoctorHospitalSchedule){
		func(s *DoctorHospitalSchedule) { s.Status = "PENDING" },
		func(s *DoctorHospitalSchedule) { s.Timezone = "Asia/Makassar" },
		func(s *DoctorHospitalSchedule) { s.BookingMode = "SESSION_QUEUE" },
		func(s *DoctorHospitalSchedule) { s.Capacity = 2 },
		func(s *DoctorHospitalSchedule) { s.SlotDurationMinutes = 20 },
		func(s *DoctorHospitalSchedule) { s.StartTime, s.EndTime = "20:00", "21:00" },
	}
	for index, change := range variants {
		other := base
		other.DayOfWeek = 3
		change(&other)
		if groups := GroupSchedules([]DoctorHospitalSchedule{base, other}, "ACTIVE"); len(groups) != 2 {
			t.Fatalf("variant %d lost a session boundary or booking rule: %+v", index, groups)
		}
	}
}

func TestScheduleGroupsHandleNonconsecutiveDaysAndSpecificDates(t *testing.T) {
	for _, test := range []struct {
		days []int
		want string
	}{
		{[]int{0, 6}, "Sabtu–Minggu"},
		{[]int{5, 1, 3}, "Senin, Rabu, Jumat"},
		{[]int{0, 1, 2, 3, 4, 5, 6}, "Senin–Minggu"},
		{[]int{1, 0}, "Senin, Minggu"},
	} {
		rows := []DoctorHospitalSchedule{}
		for _, day := range test.days {
			row := recurringDisplayFixture()[0]
			row.DayOfWeek = day
			rows = append(rows, row)
		}
		if group := GroupSchedules(rows, "")[0]; group.DayLabel != test.want {
			t.Fatalf("days %v: got %q, want %q", test.days, group.DayLabel, test.want)
		}
	}
	date := "2026-09-30"
	first, second := recurringDisplayFixture()[0], recurringDisplayFixture()[0]
	first.ID, second.ID = "specific-one", "specific-two"
	first.ScheduleDate, second.ScheduleDate = &date, &date
	groups := GroupSchedules([]DoctorHospitalSchedule{first, second}, "PENDING")
	if len(groups) != 2 || groups[0].DisplayLabel != "30 Sep 2026, 19:00–20:00" ||
		groups[0].Type != "SPECIFIC" || len(groups[0].DayOfWeek) != 0 {
		t.Fatalf("specific rows must keep their separate identities: %+v", groups)
	}
}

func TestScheduleGroupsAreExposedOnEveryScheduleCollection(t *testing.T) {
	rows := recurringDisplayFixture()
	for _, value := range []any{
		DoctorHospitalInvitation{ID: "invitation", Schedules: rows},
		HospitalDoctor{AffiliationID: "affiliation", Schedules: rows},
		PendingScheduleChange{ID: "pending", Status: "PENDING", Schedules: rows},
		ScheduleChangeRequest{ID: "proposal", Status: "PENDING", Schedules: rows},
		PublicDoctorAffiliation{AffiliationID: "affiliation", Schedules: rows},
		DoctorHospitalAffiliationDetail{HospitalDoctor: HospitalDoctor{Schedules: rows}},
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Schedules      []json.RawMessage `json:"schedules"`
			ScheduleGroups []ScheduleGroup   `json:"schedule_groups"`
		}
		if err := json.Unmarshal(encoded, &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Schedules) != 4 || len(body.ScheduleGroups) != 1 || body.ScheduleGroups[0].DayLabel != "Senin–Kamis" || len(body.ScheduleGroups[0].Schedules) != 4 {
			t.Fatalf("%T has missing source rows or grouping: %s", value, encoded)
		}
	}
}

func TestAffiliationGroupingPreservesHospitalInvitationAndPendingProposals(t *testing.T) {
	value := DoctorHospitalAffiliationDetail{
		HospitalDoctor: HospitalDoctor{
			DoctorID: "doctor", DoctorMedikaOneID: "MDO-1111111111114111", AffiliationID: "affiliation",
			Schedules:              recurringDisplayFixture(),
			PendingScheduleChanges: []PendingScheduleChange{{ID: "pending", Status: "PENDING", Schedules: recurringDisplayFixture()}},
		},
		Hospital: &HospitalInformation{ID: "hospital"}, Invitation: AffiliationInvitation{ID: "invitation"},
		InvitationID: "private-query-field",
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		DoctorID               string                `json:"doctor_id"`
		DoctorMedikaOneID      string                `json:"doctor_medikaone_id"`
		Hospital               HospitalInformation   `json:"hospital"`
		Invitation             AffiliationInvitation `json:"invitation"`
		ScheduleGroups         []ScheduleGroup       `json:"schedule_groups"`
		PendingScheduleChanges []struct {
			ScheduleGroups []ScheduleGroup `json:"schedule_groups"`
		} `json:"pending_schedule_changes"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if body.DoctorID != "doctor" || body.DoctorMedikaOneID == "" || body.Hospital.ID != "hospital" ||
		body.Invitation.ID != "invitation" || len(body.PendingScheduleChanges) != 1 ||
		body.ScheduleGroups[0].Status != "ACTIVE" || body.PendingScheduleChanges[0].ScheduleGroups[0].Status != "PENDING" {
		t.Fatalf("affiliation lost context or merged active and pending: %s", encoded)
	}
	if body.ScheduleGroups[0].Schedules[0].Status != "ACTIVE" || body.PendingScheduleChanges[0].ScheduleGroups[0].Schedules[0].Status != "PENDING" {
		t.Fatalf("nested rows lost active/proposal status: %s", encoded)
	}
}

func TestNestedScheduleRowsPreserveDatesTargetsAndBookingFields(t *testing.T) {
	date, target := "2026-09-30", "active-target"
	rows := []DoctorHospitalSchedule{
		{ID: "proposal-sunday", DayOfWeek: 0, StartTime: "08:00", EndTime: "09:00", Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 20, Capacity: 2},
		{ID: "proposal-specific", TargetScheduleID: &target, DayOfWeek: 3, ScheduleDate: &date, StartTime: "19:00", EndTime: "20:00", Timezone: "Asia/Makassar", BookingMode: "SESSION_QUEUE", SlotDurationMinutes: 30, Capacity: 20},
	}
	encoded, err := json.Marshal(GroupSchedules(rows, "PENDING"))
	if err != nil {
		t.Fatal(err)
	}
	var groups []struct {
		Schedules []struct {
			ID                  string  `json:"id"`
			TargetScheduleID    *string `json:"target_schedule_id"`
			Status              string  `json:"status"`
			DayOfWeek           []int   `json:"day_of_week"`
			ScheduleDate        *string `json:"schedule_date"`
			StartTime           string  `json:"start_time"`
			EndTime             string  `json:"end_time"`
			Timezone            string  `json:"timezone"`
			BookingMode         string  `json:"booking_mode"`
			SlotDurationMinutes int     `json:"slot_duration_minutes"`
			Capacity            int     `json:"capacity"`
		} `json:"schedules"`
	}
	if err := json.Unmarshal(encoded, &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || len(groups[0].Schedules) != 1 || len(groups[1].Schedules) != 1 {
		t.Fatalf("row lost or combined across kinds: %s", encoded)
	}
	for i, group := range groups {
		got, want := group.Schedules[0], rows[i]
		if got.ID != want.ID || got.Status != "PENDING" || !reflect.DeepEqual(got.TargetScheduleID, want.TargetScheduleID) ||
			!reflect.DeepEqual(got.ScheduleDate, want.ScheduleDate) || got.StartTime != want.StartTime || got.EndTime != want.EndTime ||
			got.Timezone != want.Timezone || got.BookingMode != want.BookingMode || got.SlotDurationMinutes != want.SlotDurationMinutes || got.Capacity != want.Capacity {
			t.Fatalf("nested row lost metadata: %s", encoded)
		}
	}
	if !reflect.DeepEqual(groups[0].Schedules[0].DayOfWeek, []int{0}) || !reflect.DeepEqual(groups[1].Schedules[0].DayOfWeek, []int{}) {
		t.Fatalf("weekday array changed at nested JSON boundary: %s", encoded)
	}
}

func TestEmptyScheduleGroupsAreArrays(t *testing.T) {
	groups, err := json.Marshal(GroupSchedules(nil, ""))
	if err != nil || string(groups) != "[]" {
		t.Fatalf("empty group must be []: %s, %v", groups, err)
	}
}
