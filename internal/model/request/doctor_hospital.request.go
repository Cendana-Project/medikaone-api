package request

type CreateHospitalDepartmentRequest struct {
	Code string `json:"code" validate:"required,max=40"`
	Name string `json:"name" validate:"required,max=120"`
}

type CreateHospitalRoomRequest struct {
	DepartmentID string `json:"department_id" validate:"required,uuid"`
	Code         string `json:"code" validate:"required,max=40"`
	Name         string `json:"name" validate:"required,max=120"`
}

type DoctorSearchQuery struct {
	Email       string `form:"email"`
	SIPNumber   string `form:"sip_number"`
	MedikaOneID string `form:"medikaone_id"`
}

type DoctorInvitationScheduleRequest struct {
	DayOfWeek        int    `json:"day_of_week"`
	StartTime        string `json:"start_time"`
	EndTime          string `json:"end_time"`
	Timezone         string `json:"timezone,omitempty"`
	BookingMode      string `json:"booking_mode,omitempty"`
	SlotDurationMins int    `json:"slot_duration_minutes,omitempty"`
	Capacity         int    `json:"capacity,omitempty"`
}

type CreateDoctorHospitalInvitationRequest struct {
	DoctorID     string                            `json:"doctor_id"`
	DepartmentID string                            `json:"department_id"`
	RoomID       *string                           `json:"room_id,omitempty"`
	Message      *string                           `json:"message,omitempty"`
	Schedules    []DoctorInvitationScheduleRequest `json:"schedules"`
}

type RejectDoctorHospitalInvitationRequest struct {
	Reason *string `json:"reason,omitempty" validate:"omitempty,max=500"`
}

type UpdateDoctorHospitalAffiliationStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=ACTIVE SUSPENDED"`
}
