package response

import "time"

type AvailabilityDoctor struct {
	DoctorID          string `json:"doctor_id"`
	DoctorMedikaOneID string `json:"doctor_medikaone_id" gorm:"column:doctor_medikaone_id"`
	DoctorName        string `json:"doctor_name"`
}

type GroupedDoctorAvailability struct {
	AvailabilityDoctor
	DateFrom string             `json:"date_from"`
	DateTo   string             `json:"date_to"`
	Dates    []AvailabilityDate `json:"dates"`
}

type AvailabilityDate struct {
	Date              string                 `json:"date"`
	HasAvailableSlots bool                   `json:"has_available_slots"`
	Hospitals         []AvailabilityHospital `json:"hospitals"`
}

type AvailabilityHospital struct {
	HospitalID        string                    `json:"hospital_id"`
	HospitalName      string                    `json:"hospital_name"`
	HasAvailableSlots bool                      `json:"has_available_slots"`
	Slots             []GroupedAvailabilitySlot `json:"slots"`
}

type GroupedAvailabilitySlot struct {
	ScheduleID          string    `json:"schedule_id"`
	AffiliationID       string    `json:"affiliation_id"`
	DepartmentID        string    `json:"department_id"`
	DepartmentName      string    `json:"department_name"`
	RoomID              *string   `json:"room_id,omitempty"`
	RoomName            *string   `json:"room_name,omitempty"`
	ScheduleType        string    `json:"schedule_type"`
	BookingMode         string    `json:"booking_mode"`
	StartTime           string    `json:"start_time"`
	EndTime             string    `json:"end_time"`
	Timezone            string    `json:"timezone"`
	StartAt             time.Time `json:"start_at"`
	EndAt               time.Time `json:"end_at"`
	SlotDurationMinutes int       `json:"slot_duration_minutes"`
	Capacity            int       `json:"capacity"`
	AvailableCapacity   int       `json:"available_capacity"`
	IsBookable          bool      `json:"is_bookable"`
	Status              string    `json:"status"`
	UnavailableReason   *string   `json:"unavailable_reason"`
}
