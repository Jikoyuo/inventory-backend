package model

import (
	"time"

	"github.com/google/uuid"
)

// Shift represents a work schedule assignment for a user
// Supports overnight shifts (e.g., 22:00 - 06:00 next day)
type Shift struct {
	BaseModel
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id" validate:"uuid_required"`
	User   *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`

	// Time specification (HH:MM format, stored as string for minute precision)
	// For overnight shifts: StartTime > EndTime (e.g., 22:00 - 06:00)
	StartTime string `gorm:"type:varchar(5);not null" json:"start_time" validate:"required"`
	EndTime   string `gorm:"type:varchar(5);not null" json:"end_time" validate:"required"`

	// Date range (inclusive)
	StartDate time.Time `gorm:"type:date;not null;index" json:"start_date" validate:"required"`
	EndDate   time.Time `gorm:"type:date;not null;index" json:"end_date" validate:"required"`

	// Flag to indicate if this is an overnight shift (calculated on save)
	IsOvernight bool `gorm:"default:false" json:"is_overnight"`

	// Optional notes
	Note string `gorm:"type:text" json:"note,omitempty"`

	// Denormalized for quick lookup (calculated on save)
	TotalDays int `gorm:"not null" json:"total_days"`
}

// TableName specifies the table name for GORM
func (Shift) TableName() string {
	return "shifts"
}

// ShiftResponse for API responses
type ShiftResponse struct {
	ID          uuid.UUID     `json:"id"`
	UserID      uuid.UUID     `json:"user_id"`
	User        *UserResponse `json:"user,omitempty"`
	StartTime   string        `json:"start_time"`
	EndTime     string        `json:"end_time"`
	StartDate   string        `json:"start_date"`
	EndDate     string        `json:"end_date"`
	IsOvernight bool          `json:"is_overnight"`
	Note        string        `json:"note,omitempty"`
	TotalDays   int           `json:"total_days"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	CreatedBy   string        `json:"created_by"`
	UpdatedBy   string        `json:"updated_by"`
}

// ToResponse converts Shift to ShiftResponse
func (s *Shift) ToResponse() ShiftResponse {
	response := ShiftResponse{
		ID:          s.ID,
		UserID:      s.UserID,
		StartTime:   s.StartTime,
		EndTime:     s.EndTime,
		StartDate:   s.StartDate.Format("2006-01-02"),
		EndDate:     s.EndDate.Format("2006-01-02"),
		IsOvernight: s.IsOvernight,
		Note:        s.Note,
		TotalDays:   s.TotalDays,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
		CreatedBy:   s.CreatedBy,
		UpdatedBy:   s.UpdatedBy,
	}

	if s.User != nil {
		userResp := s.User.ToResponse()
		response.User = &userResp
	}

	return response
}

// ViewType for filtering shifts
type ViewType string

const (
	ViewTypeDaily   ViewType = "daily"
	ViewTypeWeekly  ViewType = "weekly"
	ViewTypeMonthly ViewType = "monthly"
	ViewTypeAll     ViewType = "all"
)
