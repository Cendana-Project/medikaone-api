package entity

import "time"

type UserRole struct {
	ID        string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex:ux_user_roles_user_role" json:"user_id"`
	RoleID    string    `gorm:"type:uuid;not null;uniqueIndex:ux_user_roles_user_role" json:"role_id"`
	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
}
