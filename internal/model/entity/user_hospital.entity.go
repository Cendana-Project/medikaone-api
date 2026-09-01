package entity

import "time"

type UserHospital struct {
	ID         string     `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID     string     `gorm:"type:uuid;not null;index" json:"user_id"`
	HospitalID string     `gorm:"type:uuid;not null;index" json:"hospital_id"`
	IsActive   bool       `gorm:"type:boolean;default:true" json:"is_active"`
	IsPrimary  bool       `gorm:"type:boolean;default:false" json:"is_primary"`
	CreatedAt  time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}
