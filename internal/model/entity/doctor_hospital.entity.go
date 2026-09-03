package entity

import "time"

const (
	DoctorHospitalInvitationPending   = "PENDING"
	DoctorHospitalInvitationAccepted  = "ACCEPTED"
	DoctorHospitalInvitationRejected  = "REJECTED"
	DoctorHospitalInvitationCancelled = "CANCELLED"
	DoctorHospitalInvitationExpired   = "EXPIRED"

	DoctorHospitalAffiliationActive    = "ACTIVE"
	DoctorHospitalAffiliationSuspended = "SUSPENDED"
)

type HospitalDepartment struct {
	ID         string    `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	HospitalID string    `json:"hospital_id" gorm:"type:uuid;not null;index"`
	Code       string    `json:"code" gorm:"type:varchar(40);not null"`
	Name       string    `json:"name" gorm:"type:varchar(120);not null"`
	IsActive   bool      `json:"is_active" gorm:"not null;default:true"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type HospitalRoom struct {
	ID           string    `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	HospitalID   string    `json:"hospital_id" gorm:"type:uuid;not null;index"`
	DepartmentID string    `json:"department_id" gorm:"type:uuid;not null;index"`
	Code         string    `json:"code" gorm:"type:varchar(40);not null"`
	Name         string    `json:"name" gorm:"type:varchar(120);not null"`
	IsActive     bool      `json:"is_active" gorm:"not null;default:true"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DoctorHospitalInvitation struct {
	ID              string     `json:"id"`
	HospitalID      string     `json:"hospital_id"`
	DoctorID        string     `json:"doctor_id"`
	DepartmentID    string     `json:"department_id"`
	RoomID          *string    `json:"room_id,omitempty"`
	InvitedBy       string     `json:"invited_by"`
	SupersedesID    *string    `json:"supersedes_invitation_id,omitempty" gorm:"column:supersedes_invitation_id"`
	Status          string     `json:"status"`
	Message         *string    `json:"message,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at"`
	RespondedAt     *time.Time `json:"responded_at,omitempty"`
	CancelledAt     *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type DoctorHospitalAffiliation struct {
	ID           string    `json:"id"`
	HospitalID   string    `json:"hospital_id"`
	DoctorID     string    `json:"doctor_id"`
	DepartmentID string    `json:"department_id"`
	RoomID       *string   `json:"room_id,omitempty"`
	InvitationID string    `json:"invitation_id"`
	Status       string    `json:"status"`
	JoinedAt     time.Time `json:"joined_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Notification struct {
	ID        string     `json:"id"`
	UserID    string     `json:"-"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Data      []byte     `json:"data" gorm:"type:jsonb"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
