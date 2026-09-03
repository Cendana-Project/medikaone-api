package response

import (
	"encoding/json"
	"time"
)

type DoctorSearchResult struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	SIPNumber string `json:"sip_number"`
	Specialty string `json:"specialty"`
}

type DoctorHospitalSchedule struct {
	ID                  string `json:"id,omitempty"`
	DayOfWeek           int    `json:"day_of_week"`
	StartTime           string `json:"start_time"`
	EndTime             string `json:"end_time"`
	Timezone            string `json:"timezone"`
	BookingMode         string `json:"booking_mode"`
	SlotDurationMinutes int    `json:"slot_duration_minutes"`
	Capacity            int    `json:"capacity"`
}

type DoctorHospitalInvitation struct {
	ID                 string                   `json:"id"`
	HospitalID         string                   `json:"hospital_id"`
	HospitalCode       string                   `json:"hospital_code"`
	HospitalName       string                   `json:"hospital_name"`
	DoctorID           string                   `json:"doctor_id"`
	DoctorEmail        string                   `json:"doctor_email"`
	DoctorFirstName    string                   `json:"doctor_first_name"`
	DoctorLastName     string                   `json:"doctor_last_name"`
	SIPNumber          string                   `json:"sip_number"`
	Specialty          string                   `json:"specialty"`
	DepartmentID       string                   `json:"department_id"`
	DepartmentName     string                   `json:"department_name"`
	RoomID             *string                  `json:"room_id,omitempty"`
	RoomName           *string                  `json:"room_name,omitempty"`
	InvitedBy          string                   `json:"invited_by"`
	SupersedesID       *string                  `json:"supersedes_invitation_id,omitempty" gorm:"column:supersedes_invitation_id"`
	Status             string                   `json:"status"`
	Message            *string                  `json:"message,omitempty"`
	RejectionReason    *string                  `json:"rejection_reason,omitempty"`
	ExpiresAt          time.Time                `json:"expires_at"`
	RespondedAt        *time.Time               `json:"responded_at,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	ContractFilename   string                   `json:"contract_filename"`
	SignedContractName *string                  `json:"signed_contract_filename,omitempty"`
	Schedules          []DoctorHospitalSchedule `json:"schedules"`
}

type HospitalDoctor struct {
	AffiliationID string                   `json:"affiliation_id"`
	HospitalID    string                   `json:"hospital_id"`
	HospitalName  string                   `json:"hospital_name"`
	DoctorID      string                   `json:"doctor_id"`
	Email         string                   `json:"email"`
	FirstName     string                   `json:"first_name"`
	LastName      string                   `json:"last_name"`
	SIPNumber     string                   `json:"sip_number"`
	Specialty     string                   `json:"specialty"`
	DepartmentID  string                   `json:"department_id"`
	Department    string                   `json:"department"`
	RoomID        *string                  `json:"room_id,omitempty"`
	Room          *string                  `json:"room,omitempty"`
	Status        string                   `json:"status"`
	JoinedAt      time.Time                `json:"joined_at"`
	Schedules     []DoctorHospitalSchedule `json:"schedules"`
}

type Notification struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data"`
	ReadAt    *time.Time      `json:"read_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}
