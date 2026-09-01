package entity

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID           string         `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	SeedKey      *string        `json:"-" gorm:"type:varchar(128)"`
	Email        string         `json:"email" gorm:"type:varchar(190);not null"`
	Username     *string        `json:"username" gorm:"type:varchar(64)"`
	FirstName    string         `json:"first_name"`
	LastName     string         `json:"last_name"`
	Phone        *string        `json:"phone"`
	DOB          *time.Time     `json:"dob"`
	Address      *string        `json:"address"`
	Gender       *string        `json:"gender"` // L|P
	NIK          *string        `json:"nik"`
	PasswordHash string         `json:"-" gorm:"not null"`
	Status       string         `json:"status" gorm:"type:varchar(16);not null;default:'pending'"`
	VerifiedAt   *time.Time     `json:"verified_at"`
	CreatedAt    time.Time      `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt    *time.Time     `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}
