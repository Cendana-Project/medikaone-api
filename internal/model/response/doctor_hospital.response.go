package response

import (
	"encoding/json"
	"time"
)

type DoctorSearchResult struct {
	DoctorMedikaOneID string `json:"doctor_medikaone_id" gorm:"column:doctor_medikaone_id"`
	ID                string `json:"id"`
	Email             string `json:"email"`
	Username          string `json:"username"`
	Phone             string `json:"phone"`
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	SIPNumber         string `json:"sip_number" gorm:"column:sip_number"`
	Specialty         string `json:"specialty"`
}

type DoctorHospitalSchedule struct {
	ID                  string  `json:"id,omitempty"`
	Status              string  `json:"status,omitempty"`
	DayOfWeek           int     `json:"-"`
	ScheduleDate        *string `json:"schedule_date,omitempty"`
	StartTime           string  `json:"start_time"`
	EndTime             string  `json:"end_time"`
	Timezone            string  `json:"timezone"`
	BookingMode         string  `json:"booking_mode"`
	SlotDurationMinutes int     `json:"slot_duration_minutes"`
	Capacity            int     `json:"capacity"`
}

// PendingScheduleChange keeps a proposal separate from the currently active
// schedules. Schedule item IDs belong to the proposal until it is approved;
// approval creates new active schedule IDs.
type PendingScheduleChange struct {
	ID               string                   `json:"id"`
	AffiliationID    string                   `json:"-"`
	Operation        string                   `json:"operation"`
	TargetScheduleID *string                  `json:"target_schedule_id,omitempty"`
	RequestedBy      string                   `json:"requested_by"`
	RequestedByParty string                   `json:"requested_by_party"`
	Status           string                   `json:"status"`
	Reason           *string                  `json:"reason,omitempty"`
	ExpiresAt        time.Time                `json:"expires_at"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
	Schedules        []DoctorHospitalSchedule `json:"schedules" gorm:"-"`
}

// HospitalInformation is the hospital snapshot exposed by invitation and
// affiliation details. It intentionally excludes memberships and storage
// internals while keeping the information needed by the doctor offer screen.
type HospitalInformation struct {
	ID              string          `json:"id"`
	Code            *string         `json:"code"`
	Name            string          `json:"name"`
	Address         *string         `json:"address"`
	City            *string         `json:"city"`
	Province        *string         `json:"province"`
	Country         *string         `json:"country"`
	Latitude        *float64        `json:"latitude"`
	Longitude       *float64        `json:"longitude"`
	Phone           *string         `json:"phone"`
	Email           *string         `json:"email"`
	Website         *string         `json:"website"`
	Description     *string         `json:"description"`
	Facilities      json.RawMessage `json:"facilities"`
	EstablishedYear *int            `json:"established_year"`
	Timezone        string          `json:"timezone"`
	OpeningHours    json.RawMessage `json:"opening_hours"`
	RatingAverage   *float64        `json:"rating_average"`
	RatingCount     int64           `json:"rating_count"`
	IsActive        bool            `json:"is_active"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type DoctorHospitalInvitation struct {
	DoctorMedikaOneID  string                   `json:"doctor_medikaone_id" gorm:"column:doctor_medikaone_id"`
	ID                 string                   `json:"id"`
	HospitalID         string                   `json:"hospital_id"`
	HospitalCode       string                   `json:"hospital_code"`
	HospitalName       string                   `json:"hospital_name"`
	DoctorID           string                   `json:"doctor_id"`
	DoctorEmail        string                   `json:"doctor_email"`
	DoctorFirstName    string                   `json:"doctor_first_name"`
	DoctorLastName     string                   `json:"doctor_last_name"`
	SIPNumber          string                   `json:"sip_number" gorm:"column:sip_number"`
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
	Schedules          []DoctorHospitalSchedule `json:"schedules" gorm:"-"`
	Hospital           *HospitalInformation     `json:"hospital,omitempty" gorm:"-"`
}

type HospitalDoctor struct {
	DoctorMedikaOneID      string                   `json:"doctor_medikaone_id" gorm:"column:doctor_medikaone_id"`
	AffiliationID          string                   `json:"affiliation_id"`
	HospitalID             string                   `json:"hospital_id"`
	HospitalName           string                   `json:"hospital_name"`
	DoctorID               string                   `json:"doctor_id"`
	Email                  string                   `json:"email"`
	FirstName              string                   `json:"first_name"`
	LastName               string                   `json:"last_name"`
	SIPNumber              string                   `json:"sip_number" gorm:"column:sip_number"`
	Specialty              string                   `json:"specialty"`
	DepartmentID           string                   `json:"department_id"`
	Department             string                   `json:"department"`
	RoomID                 *string                  `json:"room_id,omitempty"`
	Room                   *string                  `json:"room,omitempty"`
	Status                 string                   `json:"status"`
	JoinedAt               time.Time                `json:"joined_at"`
	Schedules              []DoctorHospitalSchedule `json:"schedules" gorm:"-"`
	PendingScheduleChanges []PendingScheduleChange  `json:"pending_schedule_changes" gorm:"-"`
}

// AffiliationInvitation links an accepted affiliation to the originating
// offer without carrying the invitation message into the affiliation detail.
type AffiliationInvitation struct {
	ID                     string     `json:"id"`
	Status                 string     `json:"status"`
	ContractFilename       string     `json:"contract_filename"`
	SignedContractFilename *string    `json:"signed_contract_filename,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	RespondedAt            *time.Time `json:"responded_at,omitempty"`
}

type DoctorHospitalAffiliationDetail struct {
	HospitalDoctor
	Hospital                     *HospitalInformation  `json:"hospital" gorm:"-"`
	Invitation                   AffiliationInvitation `json:"invitation" gorm:"-"`
	InvitationID                 string                `json:"-"`
	InvitationStatus             string                `json:"-"`
	InvitationContractFilename   string                `json:"-"`
	InvitationSignedContractName *string               `json:"-"`
	InvitationCreatedAt          time.Time             `json:"-"`
	InvitationRespondedAt        *time.Time            `json:"-"`
}

type DoctorAffiliationStatus struct {
	DoctorID          string `json:"doctor_id"`
	DoctorMedikaOneID string `json:"doctor_medikaone_id" gorm:"column:doctor_medikaone_id"`
	Status            string `json:"status"`
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
