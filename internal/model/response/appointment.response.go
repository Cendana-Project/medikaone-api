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
	ID                     string     `json:"id"`
	AppointmentNumber      string     `json:"appointment_number"`
	PatientID              *string    `json:"patient_id,omitempty"`
	PatientRecordID        string     `json:"patient_record_id"`
	PatientName            string     `json:"patient_name"`
	AffiliationID          string     `json:"affiliation_id"`
	ScheduleID             string     `json:"schedule_id"`
	HospitalID             string     `json:"hospital_id"`
	HospitalName           string     `json:"hospital_name"`
	DoctorID               string     `json:"doctor_id"`
	DoctorName             string     `json:"doctor_name"`
	DepartmentID           string     `json:"department_id"`
	DepartmentName         string     `json:"department_name"`
	RoomID                 *string    `json:"room_id,omitempty"`
	RoomName               *string    `json:"room_name,omitempty"`
	AppointmentDate        string     `json:"appointment_date"`
	ScheduledStartAt       time.Time  `json:"scheduled_start_at"`
	ScheduledEndAt         time.Time  `json:"scheduled_end_at"`
	Timezone               string     `json:"timezone"`
	BookingMode            string     `json:"booking_mode"`
	QueueNumber            string     `json:"queue_number"`
	QueueActive            bool       `json:"queue_active"`
	Status                 string     `json:"status"`
	AttendanceStatus       string     `json:"attendance_status"`
	Source                 string     `json:"source"`
	ReasonForVisit         string     `json:"reason_for_visit,omitempty"`
	Note                   *string    `json:"note,omitempty"`
	ConsentVersion         string     `json:"consent_version,omitempty"`
	ConsentedAt            time.Time  `json:"consented_at"`
	VerificationCode       string     `json:"verification_code,omitempty"`
	QRPayload              string     `json:"qr_payload,omitempty"`
	CheckInMethod          *string    `json:"check_in_method,omitempty"`
	CheckInOverrideReason  *string    `json:"check_in_override_reason,omitempty"`
	CapacityOverridden     bool       `json:"capacity_overridden"`
	CapacityOverrideReason *string    `json:"capacity_override_reason,omitempty"`
	CheckedInAt            *time.Time `json:"checked_in_at,omitempty"`
	CancelledAt            *time.Time `json:"cancelled_at,omitempty"`
	CancellationReason     *string    `json:"cancellation_reason,omitempty"`
	CompletedAt            *time.Time `json:"completed_at,omitempty"`
	RescheduledFromID      *string    `json:"rescheduled_from_id,omitempty"`
	RescheduledToID        *string    `json:"rescheduled_to_id,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type CheckInPatientPreview struct {
	PatientRecordID      string  `json:"patient_record_id"`
	MedikaOneID          *string `json:"medikaone_id,omitempty"`
	FullName             string  `json:"full_name"`
	DateOfBirth          string  `json:"date_of_birth"`
	Gender               string  `json:"gender"`
	IdentityType         string  `json:"identity_type"`
	IdentityNumberMasked string  `json:"identity_number_masked"`
	PhoneMasked          string  `json:"phone_masked"`
	EmailMasked          *string `json:"email_masked,omitempty"`
}

type CheckInCandidate struct {
	Appointment          Appointment           `json:"appointment"`
	Patient              CheckInPatientPreview `json:"patient"`
	CheckInToken         string                `json:"check_in_token"`
	TokenExpiresAt       time.Time             `json:"token_expires_at"`
	LookupMethod         string                `json:"lookup_method"`
	LateOverrideRequired bool                  `json:"late_override_required"`
}

type CheckInLookupResult struct {
	Candidates []CheckInCandidate `json:"candidates"`
	Count      int                `json:"count"`
}

type PatientRecord struct {
	ID                   string     `json:"id"`
	UserID               *string    `json:"user_id,omitempty"`
	FirstName            string     `json:"first_name"`
	LastName             string     `json:"last_name"`
	FullName             string     `json:"full_name"`
	EmailMasked          *string    `json:"email_masked,omitempty"`
	PhoneMasked          string     `json:"phone_masked"`
	DateOfBirth          string     `json:"date_of_birth"`
	Gender               string     `json:"gender"`
	IdentityType         string     `json:"identity_type"`
	IdentityNumberMasked string     `json:"identity_number_masked"`
	ClaimedAt            *time.Time `json:"claimed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

type AppointmentPage struct {
	Items []Appointment `json:"items"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
	Total int64         `json:"total"`
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
