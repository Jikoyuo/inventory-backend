package model

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// User represents an authenticated user in the system
type User struct {
	BaseModel
	Email        string      `gorm:"type:varchar(255);uniqueIndex;not null" json:"email" validate:"required,email"`
	Password     string      `gorm:"type:varchar(255);not null" json:"-"` // Hidden from JSON
	FullName     string      `gorm:"type:varchar(255)" json:"full_name" validate:"required"`
	PhoneNumber  string      `gorm:"type:varchar(20)" json:"phone_number"`
	BirthDate    *time.Time  `gorm:"type:date" json:"birth_date,omitempty"`
	RoleID       *uint       `gorm:"index" json:"role_id"`
	Role         *Role       `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	IsActive     bool        `gorm:"default:true" json:"is_active"`
	Privileges   []Privilege `gorm:"many2many:user_privileges;" json:"privileges,omitempty"`
	TokenVersion string      `gorm:"type:varchar(255);default:''" json:"-"` // For single session enforcement
	LastSeenAt   *time.Time  `json:"last_seen_at,omitempty"`                // For user presence
}

// SetPassword hashes and sets the user's password
func (u *User) SetPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

// CheckPassword verifies if the provided password matches the stored hash
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

// HasPrivilege checks if the user has a specific privilege
func (u *User) HasPrivilege(code string) bool {
	for _, p := range u.Privileges {
		if p.Code == code {
			return true
		}
	}
	return false
}

// GetPrivilegeCodes returns a slice of all privilege codes for this user
func (u *User) GetPrivilegeCodes() []string {
	codes := make([]string, len(u.Privileges))
	for i, p := range u.Privileges {
		codes[i] = p.Code
	}
	return codes
}

// UserResponse is used for API responses (without sensitive data)
type UserResponse struct {
	ID          uuid.UUID   `json:"id"`
	Email       string      `json:"email"`
	FullName    string      `json:"full_name"`
	PhoneNumber string      `json:"phone_number"`
	BirthDate   *time.Time  `json:"birth_date,omitempty"`
	RoleID      *uint       `json:"role_id,omitempty"`
	Role        *Role       `json:"role,omitempty"`
	IsActive    bool        `json:"is_active"`
	LastSeenAt  *time.Time  `json:"last_seen_at,omitempty"`
	Privileges  []Privilege `json:"privileges"`
}

// ToResponse converts User to UserResponse
func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:          u.ID,
		Email:       u.Email,
		FullName:    u.FullName,
		PhoneNumber: u.PhoneNumber,
		BirthDate:   u.BirthDate,
		RoleID:      u.RoleID,
		Role:        u.Role,
		IsActive:    u.IsActive,
		LastSeenAt:  u.LastSeenAt,
		Privileges:  u.Privileges,
	}
}
