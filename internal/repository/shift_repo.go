package repository

import (
	"time"

	"go-inventory-ws/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ShiftRepository interface {
	Create(shift *model.Shift) error
	Update(shift *model.Shift) error
	Delete(id uuid.UUID, deletedBy string) error
	FindByID(id uuid.UUID) (*model.Shift, error)
	FindByUserID(userID uuid.UUID) ([]model.Shift, error)
	FindAll() ([]model.Shift, error)

	// Overlap detection - critical for validation
	// Returns shifts that would conflict with the given time/date range for a user
	FindOverlappingShifts(userID uuid.UUID, startDate, endDate time.Time,
		startTime, endTime string, isOvernight bool, excludeID *uuid.UUID) ([]model.Shift, error)

	// Get shifts for date range (for calendar views)
	FindByDateRange(startDate, endDate time.Time) ([]model.Shift, error)

	// Get shifts for a specific user within a date range
	FindByUserIDAndDateRange(userID uuid.UUID, startDate, endDate time.Time) ([]model.Shift, error)
}

type shiftRepo struct {
	db *gorm.DB
}

func NewShiftRepo(db *gorm.DB) ShiftRepository {
	return &shiftRepo{db}
}

func (r *shiftRepo) Create(shift *model.Shift) error {
	return r.db.Create(shift).Error
}

func (r *shiftRepo) Update(shift *model.Shift) error {
	return r.db.Save(shift).Error
}

func (r *shiftRepo) Delete(id uuid.UUID, deletedBy string) error {
	return r.db.Model(&model.Shift{}).Where("id = ?", id).Updates(map[string]interface{}{
		"deleted_at": gorm.Expr("NOW()"),
		"deleted_by": deletedBy,
	}).Error
}

func (r *shiftRepo) FindByID(id uuid.UUID) (*model.Shift, error) {
	var shift model.Shift
	if err := r.db.Preload("User").Preload("User.Role").First(&shift, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &shift, nil
}

func (r *shiftRepo) FindByUserID(userID uuid.UUID) ([]model.Shift, error) {
	var shifts []model.Shift
	if err := r.db.Preload("User").Preload("User.Role").
		Where("user_id = ?", userID).
		Order("start_date ASC, start_time ASC").
		Find(&shifts).Error; err != nil {
		return nil, err
	}
	return shifts, nil
}

func (r *shiftRepo) FindAll() ([]model.Shift, error) {
	var shifts []model.Shift
	if err := r.db.Preload("User").Preload("User.Role").
		Order("start_date ASC, start_time ASC").
		Find(&shifts).Error; err != nil {
		return nil, err
	}
	return shifts, nil
}

// FindOverlappingShifts checks for schedule conflicts
// This is complex because we need to handle:
// 1. Date range overlap
// 2. Time range overlap (including overnight shifts)
func (r *shiftRepo) FindOverlappingShifts(userID uuid.UUID, startDate, endDate time.Time,
	startTime, endTime string, isOvernight bool, excludeID *uuid.UUID) ([]model.Shift, error) {

	var shifts []model.Shift

	query := r.db.Where("user_id = ?", userID).
		Where("start_date <= ? AND end_date >= ?", endDate, startDate) // Date ranges overlap

	// Exclude current shift when updating
	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}

	if err := query.Preload("User").Find(&shifts).Error; err != nil {
		return nil, err
	}

	// Filter for time overlap in Go (more precise handling of overnight shifts)
	var overlapping []model.Shift
	for _, existingShift := range shifts {
		if timeRangesOverlap(startTime, endTime, isOvernight,
			existingShift.StartTime, existingShift.EndTime, existingShift.IsOvernight) {
			overlapping = append(overlapping, existingShift)
		}
	}

	return overlapping, nil
}

// timeRangesOverlap checks if two time ranges overlap
// Handles both normal and overnight shifts
func timeRangesOverlap(start1, end1 string, overnight1 bool, start2, end2 string, overnight2 bool) bool {
	// Convert times to minutes since midnight for easier comparison
	s1 := timeToMinutes(start1)
	e1 := timeToMinutes(end1)
	s2 := timeToMinutes(start2)
	e2 := timeToMinutes(end2)

	// For overnight shifts, add 24 hours (1440 minutes) to end time
	if overnight1 {
		e1 += 1440
	}
	if overnight2 {
		e2 += 1440
	}

	// Check overlap: NOT (end1 <= start2 OR end2 <= start1)
	// Also check the "wrapped" version for overnight shifts interacting with normal shifts
	if overnight1 || overnight2 {
		// Complex case: one or both are overnight
		// We need to check multiple scenarios

		// Scenario 1: Direct overlap
		if !(e1 <= s2 || e2 <= s1) {
			return true
		}

		// Scenario 2: Overnight shift wraps around
		// If shift1 is overnight (22:00-06:00), it occupies 22:00-23:59 AND 00:00-06:00
		// We need to check if shift2 overlaps with either part
		if overnight1 && !overnight2 {
			// Check if shift2 overlaps with the "next day" part of shift1 (00:00 to end1)
			// The next day part is 0 to (e1 - 1440)
			nextDayEnd1 := e1 - 1440
			if s2 < nextDayEnd1 {
				return true
			}
		}

		if overnight2 && !overnight1 {
			// Check if shift1 overlaps with the "next day" part of shift2
			nextDayEnd2 := e2 - 1440
			if s1 < nextDayEnd2 {
				return true
			}
		}

		return false
	}

	// Simple case: neither is overnight
	return !(e1 <= s2 || e2 <= s1)
}

// timeToMinutes converts HH:MM string to minutes since midnight
func timeToMinutes(timeStr string) int {
	var hours, minutes int
	// Parse "HH:MM" format
	if len(timeStr) >= 5 {
		hours = int(timeStr[0]-'0')*10 + int(timeStr[1]-'0')
		minutes = int(timeStr[3]-'0')*10 + int(timeStr[4]-'0')
	}
	return hours*60 + minutes
}

func (r *shiftRepo) FindByDateRange(startDate, endDate time.Time) ([]model.Shift, error) {
	var shifts []model.Shift
	if err := r.db.Preload("User").Preload("User.Role").
		Where("start_date <= ? AND end_date >= ?", endDate, startDate).
		Order("start_date ASC, start_time ASC").
		Find(&shifts).Error; err != nil {
		return nil, err
	}
	return shifts, nil
}

func (r *shiftRepo) FindByUserIDAndDateRange(userID uuid.UUID, startDate, endDate time.Time) ([]model.Shift, error) {
	var shifts []model.Shift
	if err := r.db.Preload("User").Preload("User.Role").
		Where("user_id = ?", userID).
		Where("start_date <= ? AND end_date >= ?", endDate, startDate).
		Order("start_date ASC, start_time ASC").
		Find(&shifts).Error; err != nil {
		return nil, err
	}
	return shifts, nil
}
