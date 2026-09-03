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

type AppointmentTransitionRequest struct {
	Reason *string `json:"reason,omitempty" validate:"omitempty,max=1000"`
}
