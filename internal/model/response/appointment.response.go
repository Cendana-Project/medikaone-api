package response

import "time"

type AvailabilitySlot struct {
	StartAt           time.Time `json:"start_at"`
	EndAt             time.Time `json:"end_at"`
	AvailableCapacity int       `json:"available_capacity"`
	Capacity          int       `json:"capacity"`
}

type DoctorScheduleAvailability struct {
	ScheduleID          string             `json:"schedule_id"`
	AffiliationID       string             `json:"affiliation_id"`
	HospitalID          string             `json:"hospital_id"`
	HospitalName        string             `json:"hospital_name"`
	DoctorID            string             `json:"doctor_id"`
	DoctorName          string             `json:"doctor_name"`
	DepartmentID        string             `json:"department_id"`
	DepartmentName      string             `json:"department_name"`
	RoomID              *string            `json:"room_id,omitempty"`
	RoomName            *string            `json:"room_name,omitempty"`
	Date                string             `json:"date"`
	Timezone            string             `json:"timezone"`
	BookingMode         string             `json:"booking_mode"`
	SlotDurationMinutes int                `json:"slot_duration_minutes"`
	Capacity            int                `json:"capacity"`
	SessionStartAt      time.Time          `json:"session_start_at"`
	SessionEndAt        time.Time          `json:"session_end_at"`
	AvailableCapacity   int                `json:"available_capacity"`
	Slots               []AvailabilitySlot `json:"slots"`
}

type Appointment struct {
	ID                 string     `json:"id"`
	AppointmentNumber  string     `json:"appointment_number"`
	PatientID          string     `json:"patient_id"`
	PatientName        string     `json:"patient_name"`
	AffiliationID      string     `json:"affiliation_id"`
	ScheduleID         string     `json:"schedule_id"`
	HospitalID         string     `json:"hospital_id"`
	HospitalName       string     `json:"hospital_name"`
	DoctorID           string     `json:"doctor_id"`
	DoctorName         string     `json:"doctor_name"`
	DepartmentID       string     `json:"department_id"`
	DepartmentName     string     `json:"department_name"`
	RoomID             *string    `json:"room_id,omitempty"`
	RoomName           *string    `json:"room_name,omitempty"`
	AppointmentDate    string     `json:"appointment_date"`
	ScheduledStartAt   time.Time  `json:"scheduled_start_at"`
	ScheduledEndAt     time.Time  `json:"scheduled_end_at"`
	Timezone           string     `json:"timezone"`
	BookingMode        string     `json:"booking_mode"`
	QueueNumber        string     `json:"queue_number"`
	QueueActive        bool       `json:"queue_active"`
	Status             string     `json:"status"`
	AttendanceStatus   string     `json:"attendance_status"`
	ReasonForVisit     string     `json:"reason_for_visit,omitempty"`
	Note               *string    `json:"note,omitempty"`
	ConsentVersion     string     `json:"consent_version,omitempty"`
	ConsentedAt        time.Time  `json:"consented_at"`
	VerificationCode   string     `json:"verification_code,omitempty"`
	CheckedInAt        *time.Time `json:"checked_in_at,omitempty"`
	CancelledAt        *time.Time `json:"cancelled_at,omitempty"`
	CancellationReason *string    `json:"cancellation_reason,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	RescheduledFromID  *string    `json:"rescheduled_from_id,omitempty"`
	RescheduledToID    *string    `json:"rescheduled_to_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ScheduleChangeRequest struct {
	ID               string                   `json:"id"`
	AffiliationID    string                   `json:"affiliation_id"`
	HospitalID       string                   `json:"hospital_id"`
	HospitalName     string                   `json:"hospital_name"`
	DoctorID         string                   `json:"doctor_id"`
	DoctorName       string                   `json:"doctor_name"`
	RequestedBy      string                   `json:"requested_by"`
	RequestedByParty string                   `json:"requested_by_party"`
	Status           string                   `json:"status"`
	Reason           *string                  `json:"reason,omitempty"`
	ReviewedBy       *string                  `json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time               `json:"reviewed_at,omitempty"`
	RejectionReason  *string                  `json:"rejection_reason,omitempty"`
	ExpiresAt        time.Time                `json:"expires_at"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
	Schedules        []DoctorHospitalSchedule `json:"schedules"`
}

type AppointmentReminder struct {
	ID               string    `json:"id"`
	AppointmentID    string    `json:"appointment_id"`
	ReminderType     string    `json:"reminder_type"`
	DueAt            time.Time `json:"due_at"`
	PatientID        string    `json:"patient_id"`
	PatientEmail     string    `json:"patient_email"`
	PatientFirstName string    `json:"patient_first_name"`
	DoctorID         string    `json:"doctor_id"`
	DoctorEmail      string    `json:"doctor_email"`
	DoctorFirstName  string    `json:"doctor_first_name"`
	HospitalName     string    `json:"hospital_name"`
	ScheduledStartAt time.Time `json:"scheduled_start_at"`
	Timezone         string    `json:"timezone"`
}
