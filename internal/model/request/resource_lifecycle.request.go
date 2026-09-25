package request

import "encoding/json"

// Pointer fields retain the existing value when omitted. Empty optional strings
// clear the value; schedules: [] clears the proposed invitation schedules.
type UpdateHospitalRequest struct {
	Code            *string               `json:"code" validate:"omitempty,uppercase,alphanumdash,min=3,max=40"`
	Name            *string               `json:"name" validate:"omitempty,min=1,max=160"`
	Address         *string               `json:"address" validate:"omitempty,min=1,max=1000"`
	City            *string               `json:"city" validate:"omitempty,min=1,max=100"`
	Province        *string               `json:"province" validate:"omitempty,min=1,max=100"`
	Country         *string               `json:"country" validate:"omitempty,min=1,max=100"`
	Latitude        *float64              `json:"latitude"`
	Longitude       *float64              `json:"longitude"`
	Phone           *string               `json:"phone" validate:"omitempty,min=1,max=50"`
	Description     *string               `json:"description" validate:"omitempty,max=10000"`
	Facilities      json.RawMessage       `json:"facilities"`
	Email           *string               `json:"email" validate:"omitempty,max=190"`
	Website         *string               `json:"website" validate:"omitempty,max=2048"`
	EstablishedYear *int                  `json:"established_year"`
	Timezone        *string               `json:"timezone"`
	OpeningHours    *[]HospitalOpeningDay `json:"opening_hours"`
}

type UpdateHospitalDepartmentRequest struct {
	Code *string `json:"code" validate:"omitempty,min=1,max=40"`
	Name *string `json:"name" validate:"omitempty,min=1,max=120"`
}

type UpdateHospitalRoomRequest struct {
	DepartmentID *string `json:"department_id" validate:"omitempty,uuid"`
	Code         *string `json:"code" validate:"omitempty,min=1,max=40"`
	Name         *string `json:"name" validate:"omitempty,min=1,max=120"`
}

type UpdateDoctorHospitalInvitationRequest struct {
	DepartmentID *string                            `json:"department_id"`
	RoomID       *string                            `json:"room_id"`
	Message      *string                            `json:"message" validate:"omitempty,max=1000"`
	Schedules    *[]DoctorInvitationScheduleRequest `json:"schedules"`
}
