package entity

import "time"

// MasterSpecialty is a local selectable clinical field, not proof of qualification.
type MasterSpecialty struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	Code      string `gorm:"size:64;not null;uniqueIndex"`
	Name      string `gorm:"size:160;not null"`
	Category  string `gorm:"size:32;not null"`
	SortOrder int16  `gorm:"not null"`
	IsActive  bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (MasterSpecialty) TableName() string { return "master_specialties" }
