package entity

const (
	BookingModeFixedSlot    = "FIXED_SLOT"
	BookingModeSessionQueue = "SESSION_QUEUE"

	ScheduleChangePending   = "PENDING"
	ScheduleChangeApproved  = "APPROVED"
	ScheduleChangeRejected  = "REJECTED"
	ScheduleChangeCancelled = "CANCELLED"
	ScheduleChangeExpired   = "EXPIRED"

	ScheduleChangePartyDoctor   = "DOCTOR"
	ScheduleChangePartyHospital = "HOSPITAL"

	AppointmentConfirmed      = "CONFIRMED"
	AppointmentCheckedIn      = "CHECKED_IN"
	AppointmentWaitingVitals  = "WAITING_VITALS"
	AppointmentWaitingDoctor  = "WAITING_DOCTOR"
	AppointmentInConsultation = "IN_CONSULTATION"
	AppointmentCompleted      = "COMPLETED"
	AppointmentCancelled      = "CANCELLED"
	AppointmentNoShow         = "NO_SHOW"
	AppointmentRescheduled    = "RESCHEDULED"

	AttendancePending = "PENDING"
	AttendancePresent = "PRESENT"
	AttendanceNoShow  = "NO_SHOW"
)
