package entity

import "time"

type Hospital struct {
	ID          string     `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SeedKey     *string    `gorm:"type:varchar(128)" json:"-"`
	Code        *string    `gorm:"type:varchar(40)" json:"code,omitempty"`
	Name        string     `gorm:"type:varchar(160);not null" json:"name"`
	Address     *string    `gorm:"type:text" json:"address,omitempty"`
	City        *string    `gorm:"type:varchar(100)" json:"city,omitempty"`
	Province    *string    `gorm:"type:varchar(100)" json:"province,omitempty"`
	Country     *string    `gorm:"type:varchar(100);default:'Indonesia'" json:"country,omitempty"`
	Latitude    *float64   `gorm:"type:decimal(9,6)" json:"latitude,omitempty"`
	Longitude   *float64   `gorm:"type:decimal(9,6)" json:"longitude,omitempty"`
	Phone       *string    `gorm:"type:varchar(50)" json:"phone,omitempty"`
	Description *string    `gorm:"type:varchar(200)" json:"description,omitempty"`
	Facilities  []byte     `gorm:"type:jsonb" json:"facilities,omitempty"`
	IsActive    bool       `gorm:"type:boolean;default:true" json:"is_active"`
	CreatedAt   time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"not null;default:now()" json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}
