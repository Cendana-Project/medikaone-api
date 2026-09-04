package request

type CreateScheduleChangeRequest struct {
	AffiliationID string                            `json:"affiliation_id" validate:"required,uuid"`
	Reason        *string                           `json:"reason,omitempty" validate:"omitempty,max=1000"`
	Schedules     []DoctorInvitationScheduleRequest `json:"schedules" validate:"required,min=1,max=50"`
}

type ReviewScheduleChangeRequest struct {
	Reason *string `json:"reason,omitempty" validate:"omitempty,max=1000"`
}

type CreateAppointmentRequest struct {
	ScheduleID      string  `json:"schedule_id" validate:"required,uuid"`
	AppointmentDate string  `json:"appointment_date" validate:"required,datetime=2006-01-02"`
	StartTime       *string `json:"start_time,omitempty"`
	ReasonForVisit  string  `json:"reason_for_visit" validate:"required,max=2000"`
	Note            *string `json:"note,omitempty" validate:"omitempty,max=2000"`
	ConsentAccepted bool    `json:"consent_accepted" validate:"required"`
	ConsentVersion  string  `json:"consent_version" validate:"required,max=64"`
}

type CancelAppointmentRequest struct {
	Reason string `json:"reason" validate:"required,max=1000"`
}

type RescheduleAppointmentRequest struct {
	ScheduleID      string  `json:"schedule_id" validate:"required,uuid"`
	AppointmentDate string  `json:"appointment_date" validate:"required,datetime=2006-01-02"`
	StartTime       *string `json:"start_time,omitempty"`
	Reason          string  `json:"reason" validate:"required,max=1000"`
}

type VerifyAppointmentRequest struct {
	AppointmentNumber string  `json:"appointment_number" validate:"required,max=48"`
	VerificationCode  string  `json:"verification_code" validate:"required,max=128"`
	ForceLateCheckIn  bool    `json:"force_late_check_in,omitempty"`
	OverrideReason    *string `json:"override_reason,omitempty" validate:"omitempty,max=1000"`
}

// CheckInIdentity contains multiple facts supplied by a receptionist after
// asking the patient. At least two matching values are required by the service
// so a name or phone number alone cannot expose an appointment.
type CheckInIdentity struct {
	MedikaOneID    string  `json:"medikaone_id,omitempty" validate:"omitempty,uuid"`
	NIK            string  `json:"nik,omitempty" validate:"omitempty,len=16,numeric"`
	IdentityType   string  `json:"identity_type,omitempty" validate:"omitempty,oneof_ci=NIK PASSPORT OTHER MEDIKAONE_ID"`
	IdentityNumber string  `json:"identity_number,omitempty" validate:"omitempty,min=3,max=64"`
	Email          string  `json:"email,omitempty" validate:"omitempty,email,max=190"`
	Phone          string  `json:"phone,omitempty" validate:"omitempty,min=8,max=32"`
	Name           string  `json:"name,omitempty" validate:"omitempty,min=2,max=200"`
	DateOfBirth    *string `json:"date_of_birth,omitempty" validate:"omitempty,datetime=2006-01-02"`
}

// CheckInLookupRequest supports an optional QR, the human-readable appointment
// credential, or a privacy-preserving multi-field identity lookup. Exactly one
// lookup mode is accepted.
type CheckInLookupRequest struct {
	QRPayload         string           `json:"qr_payload,omitempty" validate:"omitempty,max=4096"`
	AppointmentNumber string           `json:"appointment_number,omitempty" validate:"omitempty,max=48"`
	VerificationCode  string           `json:"verification_code,omitempty" validate:"omitempty,max=128"`
	AppointmentDate   *string          `json:"appointment_date,omitempty" validate:"omitempty,datetime=2006-01-02"`
	Identity          *CheckInIdentity `json:"identity,omitempty"`
}

type ConfirmCheckInRequest struct {
	CheckInToken   string  `json:"check_in_token" validate:"required,max=4096"`
	OverrideReason *string `json:"override_reason,omitempty" validate:"omitempty,max=1000"`
}

type WalkInPatientRequest struct {
	PatientRecordID *string `json:"patient_record_id,omitempty" validate:"omitempty,uuid"`
	MedikaOneID     *string `json:"medikaone_id,omitempty" validate:"omitempty,uuid"`
	FirstName       string  `json:"first_name,omitempty" validate:"omitempty,max=100"`
	LastName        string  `json:"last_name,omitempty" validate:"omitempty,max=100"`
	Email           *string `json:"email,omitempty" validate:"omitempty,email,max=190"`
	Phone           string  `json:"phone,omitempty" validate:"omitempty,min=8,max=32"`
	DateOfBirth     string  `json:"date_of_birth,omitempty" validate:"omitempty,datetime=2006-01-02"`
	Gender          string  `json:"gender,omitempty" validate:"omitempty,oneof=L P"`
	IdentityType    string  `json:"identity_type,omitempty" validate:"omitempty,oneof_ci=NIK PASSPORT OTHER MEDIKAONE_ID"`
	IdentityNumber  string  `json:"identity_number,omitempty" validate:"omitempty,min=3,max=64"`
}

type CreateWalkInAppointmentRequest struct {
	ScheduleID             string               `json:"schedule_id" validate:"required,uuid"`
	StartTime              *string              `json:"start_time,omitempty"`
	Patient                WalkInPatientRequest `json:"patient" validate:"required"`
	ReasonForVisit         string               `json:"reason_for_visit" validate:"required,max=2000"`
	Note                   *string              `json:"note,omitempty" validate:"omitempty,max=2000"`
	ConsentVersion         string               `json:"consent_version" validate:"required,max=64"`
	CapacityOverride       bool                 `json:"capacity_override,omitempty"`
	CapacityOverrideReason *string              `json:"capacity_override_reason,omitempty" validate:"omitempty,max=1000"`
}

type ClaimPatientRecordRequest struct {
	IdentityType   string `json:"identity_type" validate:"required,oneof_ci=NIK PASSPORT OTHER MEDIKAONE_ID"`
	IdentityNumber string `json:"identity_number" validate:"required,min=3,max=64"`
	DateOfBirth    string `json:"date_of_birth" validate:"required,datetime=2006-01-02"`
}

type AppointmentTransitionRequest struct {
	Reason *string `json:"reason,omitempty" validate:"omitempty,max=1000"`
}
