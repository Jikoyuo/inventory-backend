package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go-inventory-ws/internal/model"
	"go-inventory-ws/internal/repository"
	"go-inventory-ws/internal/ws"

	"github.com/google/uuid"
)

// Asia/Jakarta timezone
var jakartaLoc *time.Location

func init() {
	var err error
	jakartaLoc, err = time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// Fallback to UTC+7 if timezone data not available
		jakartaLoc = time.FixedZone("WIB", 7*60*60)
	}
}

// Error definitions
var (
	ErrShiftNotFound         = errors.New("shift not found")
	ErrInvalidTimeFormat     = errors.New("invalid time format, use HH:MM (e.g., 08:30, 17:59)")
	ErrInvalidDateFormat     = errors.New("invalid date format, use YYYY-MM-DD")
	ErrEndDateBeforeStart    = errors.New("end date cannot be before start date")
	ErrStartDateInPast       = errors.New("start date cannot be in the past")
	ErrUserNotActive         = errors.New("cannot assign shift to inactive user")
	ErrShiftConflict         = errors.New("shift conflicts with existing schedule")
	ErrSameTimeStartEnd      = errors.New("start time and end time cannot be the same")
	ErrUnauthorizedShiftView = errors.New("you can only view your own shifts")
)

type ShiftService interface {
	CreateShift(req *CreateShiftRequest, creatorID string) (*model.Shift, error)
	UpdateShift(shiftID uuid.UUID, req *UpdateShiftRequest, updaterID string) (*model.Shift, error)
	DeleteShift(shiftID uuid.UUID, deleterID string) error
	GetShiftByID(id uuid.UUID, requesterID string, isMasterAdmin bool) (*model.ShiftResponse, error)
	GetShifts(requesterID string, isMasterAdmin bool, viewType string, referenceDate time.Time) ([]model.ShiftResponse, error)
	GetShiftsByUser(userID uuid.UUID, requesterID string, isMasterAdmin bool) ([]model.ShiftResponse, error)
}

type CreateShiftRequest struct {
	UserID    string `json:"user_id" validate:"required"`
	StartTime string `json:"start_time" validate:"required"` // HH:MM
	EndTime   string `json:"end_time" validate:"required"`   // HH:MM
	StartDate string `json:"start_date" validate:"required"` // YYYY-MM-DD
	EndDate   string `json:"end_date" validate:"required"`   // YYYY-MM-DD
	Note      string `json:"note"`
}

type UpdateShiftRequest struct {
	UserID    *string `json:"user_id"`    // Optional: reassign to different user
	StartTime *string `json:"start_time"` // HH:MM
	EndTime   *string `json:"end_time"`   // HH:MM
	StartDate *string `json:"start_date"` // YYYY-MM-DD
	EndDate   *string `json:"end_date"`   // YYYY-MM-DD
	Note      *string `json:"note"`
}

type shiftService struct {
	shiftRepo repository.ShiftRepository
	userRepo  repository.UserRepository
	wsHub     *ws.Hub
}

func NewShiftService(shiftRepo repository.ShiftRepository, userRepo repository.UserRepository, hub *ws.Hub) ShiftService {
	return &shiftService{
		shiftRepo: shiftRepo,
		userRepo:  userRepo,
		wsHub:     hub,
	}
}

// validateTimeFormat validates HH:MM format (00:00 - 23:59)
func validateTimeFormat(timeStr string) error {
	pattern := `^([01][0-9]|2[0-3]):([0-5][0-9])$`
	matched, _ := regexp.MatchString(pattern, timeStr)
	if !matched {
		return ErrInvalidTimeFormat
	}
	return nil
}

// validateDateFormat validates YYYY-MM-DD format and returns parsed date
func validateDateFormat(dateStr string) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", dateStr, jakartaLoc)
	if err != nil {
		return time.Time{}, ErrInvalidDateFormat
	}
	return parsed, nil
}

// isOvernight determines if the shift crosses midnight
func isOvernight(startTime, endTime string) bool {
	startMinutes := timeToMinutes(startTime)
	endMinutes := timeToMinutes(endTime)
	return endMinutes <= startMinutes
}

// timeToMinutes converts HH:MM to minutes since midnight
func timeToMinutes(timeStr string) int {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0
	}
	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	return hours*60 + minutes
}

func (s *shiftService) CreateShift(req *CreateShiftRequest, creatorID string) (*model.Shift, error) {
	// 1. Validate time format
	if err := validateTimeFormat(req.StartTime); err != nil {
		return nil, err
	}
	if err := validateTimeFormat(req.EndTime); err != nil {
		return nil, err
	}

	// 2. Check start time != end time
	if req.StartTime == req.EndTime {
		return nil, ErrSameTimeStartEnd
	}

	// 3. Validate and parse dates
	startDate, err := validateDateFormat(req.StartDate)
	if err != nil {
		return nil, err
	}
	endDate, err := validateDateFormat(req.EndDate)
	if err != nil {
		return nil, err
	}

	// 4. Validate date range
	if endDate.Before(startDate) {
		return nil, ErrEndDateBeforeStart
	}

	// 5. Validate start date is not in the past (compare dates only)
	today := time.Now().In(jakartaLoc).Truncate(24 * time.Hour)
	if startDate.Before(today) {
		return nil, ErrStartDateInPast
	}

	// 6. Parse and validate user ID
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	// 7. Check user exists and is active
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	if !user.IsActive {
		return nil, ErrUserNotActive
	}

	// 8. Determine if overnight shift
	overnight := isOvernight(req.StartTime, req.EndTime)

	// 9. Check for overlapping shifts
	overlapping, err := s.shiftRepo.FindOverlappingShifts(
		userID, startDate, endDate,
		req.StartTime, req.EndTime, overnight, nil,
	)
	if err != nil {
		return nil, errors.New("failed to check for shift conflicts")
	}
	if len(overlapping) > 0 {
		// Build detailed conflict message
		conflictDetails := formatConflictDetails(overlapping)
		return nil, fmt.Errorf("%w: %s", ErrShiftConflict, conflictDetails)
	}

	// 10. Calculate total days
	totalDays := int(endDate.Sub(startDate).Hours()/24) + 1

	// 11. Create shift
	shift := &model.Shift{
		UserID:      userID,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		StartDate:   startDate,
		EndDate:     endDate,
		IsOvernight: overnight,
		Note:        req.Note,
		TotalDays:   totalDays,
	}
	shift.CreatedBy = creatorID
	shift.UpdatedBy = creatorID

	if err := s.shiftRepo.Create(shift); err != nil {
		return nil, err
	}

	// 12. Reload with user data
	shift, err = s.shiftRepo.FindByID(shift.ID)
	if err != nil {
		return nil, err
	}

	// 13. Send targeted WebSocket notification to assigned user
	go s.notifyShiftCreated(shift, user)

	return shift, nil
}

func (s *shiftService) UpdateShift(shiftID uuid.UUID, req *UpdateShiftRequest, updaterID string) (*model.Shift, error) {
	// 1. Find existing shift
	shift, err := s.shiftRepo.FindByID(shiftID)
	if err != nil {
		return nil, ErrShiftNotFound
	}

	// Track original user for notification
	originalUserID := shift.UserID
	originalUser := shift.User

	// 2. Build updated values (merge with existing)
	startTime := shift.StartTime
	endTime := shift.EndTime
	startDate := shift.StartDate
	endDate := shift.EndDate
	userID := shift.UserID
	note := shift.Note

	// Apply updates if provided
	if req.StartTime != nil {
		if err := validateTimeFormat(*req.StartTime); err != nil {
			return nil, err
		}
		startTime = *req.StartTime
	}
	if req.EndTime != nil {
		if err := validateTimeFormat(*req.EndTime); err != nil {
			return nil, err
		}
		endTime = *req.EndTime
	}

	// Check start != end
	if startTime == endTime {
		return nil, ErrSameTimeStartEnd
	}

	if req.StartDate != nil {
		parsed, err := validateDateFormat(*req.StartDate)
		if err != nil {
			return nil, err
		}
		startDate = parsed
	}
	if req.EndDate != nil {
		parsed, err := validateDateFormat(*req.EndDate)
		if err != nil {
			return nil, err
		}
		endDate = parsed
	}

	// Validate date range
	if endDate.Before(startDate) {
		return nil, ErrEndDateBeforeStart
	}

	// Validate start date not in past (only if changed)
	if req.StartDate != nil {
		today := time.Now().In(jakartaLoc).Truncate(24 * time.Hour)
		if startDate.Before(today) {
			return nil, ErrStartDateInPast
		}
	}

	if req.UserID != nil {
		parsed, err := uuid.Parse(*req.UserID)
		if err != nil {
			return nil, errors.New("invalid user ID format")
		}
		// Check new user exists and is active
		newUser, err := s.userRepo.FindByID(parsed)
		if err != nil {
			return nil, ErrUserNotFound
		}
		if !newUser.IsActive {
			return nil, ErrUserNotActive
		}
		userID = parsed
	}

	if req.Note != nil {
		note = *req.Note
	}

	// 3. Determine if overnight
	overnight := isOvernight(startTime, endTime)

	// 4. Check for overlapping shifts (exclude current shift)
	overlapping, err := s.shiftRepo.FindOverlappingShifts(
		userID, startDate, endDate,
		startTime, endTime, overnight, &shiftID,
	)
	if err != nil {
		return nil, errors.New("failed to check for shift conflicts")
	}
	if len(overlapping) > 0 {
		conflictDetails := formatConflictDetails(overlapping)
		return nil, fmt.Errorf("%w: %s", ErrShiftConflict, conflictDetails)
	}

	// 5. Calculate total days
	totalDays := int(endDate.Sub(startDate).Hours()/24) + 1

	// 6. Update shift
	shift.UserID = userID
	shift.StartTime = startTime
	shift.EndTime = endTime
	shift.StartDate = startDate
	shift.EndDate = endDate
	shift.IsOvernight = overnight
	shift.Note = note
	shift.TotalDays = totalDays
	shift.UpdatedBy = updaterID

	if err := s.shiftRepo.Update(shift); err != nil {
		return nil, err
	}

	// 7. Reload with user data
	shift, err = s.shiftRepo.FindByID(shift.ID)
	if err != nil {
		return nil, err
	}

	// 8. Send targeted WebSocket notifications
	go s.notifyShiftUpdated(shift, originalUserID, originalUser)

	return shift, nil
}

func (s *shiftService) DeleteShift(shiftID uuid.UUID, deleterID string) error {
	// 1. Find existing shift
	shift, err := s.shiftRepo.FindByID(shiftID)
	if err != nil {
		return ErrShiftNotFound
	}

	// 2. Delete (soft delete)
	if err := s.shiftRepo.Delete(shiftID, deleterID); err != nil {
		return err
	}

	// 3. Notify affected user
	go s.notifyShiftDeleted(shift)

	return nil
}

func (s *shiftService) GetShiftByID(id uuid.UUID, requesterID string, isMasterAdmin bool) (*model.ShiftResponse, error) {
	shift, err := s.shiftRepo.FindByID(id)
	if err != nil {
		return nil, ErrShiftNotFound
	}

	// Check authorization: non-MASTER_ADMIN can only view their own shifts
	if !isMasterAdmin && shift.UserID.String() != requesterID {
		return nil, ErrUnauthorizedShiftView
	}

	response := shift.ToResponse()
	return &response, nil
}

func (s *shiftService) GetShifts(requesterID string, isMasterAdmin bool, viewType string, referenceDate time.Time) ([]model.ShiftResponse, error) {
	var shifts []model.Shift
	var err error

	// Calculate date range based on view type
	startDate, endDate := calculateDateRange(viewType, referenceDate)

	if isMasterAdmin {
		// MASTER_ADMIN sees all shifts
		if viewType == string(model.ViewTypeAll) {
			shifts, err = s.shiftRepo.FindAll()
		} else {
			shifts, err = s.shiftRepo.FindByDateRange(startDate, endDate)
		}
	} else {
		// Regular users see only their own shifts
		userID, parseErr := uuid.Parse(requesterID)
		if parseErr != nil {
			return nil, errors.New("invalid requester ID")
		}
		if viewType == string(model.ViewTypeAll) {
			shifts, err = s.shiftRepo.FindByUserID(userID)
		} else {
			shifts, err = s.shiftRepo.FindByUserIDAndDateRange(userID, startDate, endDate)
		}
	}

	if err != nil {
		return nil, err
	}

	responses := make([]model.ShiftResponse, len(shifts))
	for i, shift := range shifts {
		responses[i] = shift.ToResponse()
	}

	return responses, nil
}

func (s *shiftService) GetShiftsByUser(userID uuid.UUID, requesterID string, isMasterAdmin bool) ([]model.ShiftResponse, error) {
	// Check authorization
	if !isMasterAdmin && userID.String() != requesterID {
		return nil, ErrUnauthorizedShiftView
	}

	shifts, err := s.shiftRepo.FindByUserID(userID)
	if err != nil {
		return nil, err
	}

	responses := make([]model.ShiftResponse, len(shifts))
	for i, shift := range shifts {
		responses[i] = shift.ToResponse()
	}

	return responses, nil
}

// calculateDateRange calculates start and end dates based on view type
func calculateDateRange(viewType string, referenceDate time.Time) (time.Time, time.Time) {
	ref := referenceDate.In(jakartaLoc)

	switch model.ViewType(viewType) {
	case model.ViewTypeDaily:
		// Just the reference date
		start := ref.Truncate(24 * time.Hour)
		end := start.Add(24*time.Hour - time.Second)
		return start, end

	case model.ViewTypeWeekly:
		// Get start of week (Monday)
		weekday := int(ref.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday = 7
		}
		start := ref.Truncate(24*time.Hour).AddDate(0, 0, -(weekday - 1))
		end := start.AddDate(0, 0, 7).Add(-time.Second)
		return start, end

	case model.ViewTypeMonthly:
		// First and last day of month
		start := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, jakartaLoc)
		end := start.AddDate(0, 1, 0).Add(-time.Second)
		return start, end

	default:
		// All time: very wide range
		start := time.Date(2000, 1, 1, 0, 0, 0, 0, jakartaLoc)
		end := time.Date(2100, 12, 31, 23, 59, 59, 0, jakartaLoc)
		return start, end
	}
}

// formatConflictDetails creates a detailed message about conflicting shifts
func formatConflictDetails(shifts []model.Shift) string {
	if len(shifts) == 0 {
		return ""
	}

	details := make([]string, len(shifts))
	for i, shift := range shifts {
		details[i] = fmt.Sprintf("[%s - %s, %s to %s]",
			shift.StartTime, shift.EndTime,
			shift.StartDate.Format("2006-01-02"),
			shift.EndDate.Format("2006-01-02"))
	}

	return strings.Join(details, ", ")
}

// WebSocket notification methods

func (s *shiftService) notifyShiftCreated(shift *model.Shift, user *model.User) {
	payload := map[string]interface{}{
		"type":   "shift_notification",
		"action": "shift_created",
		"message": fmt.Sprintf("You have been assigned a new shift: %s - %s, from %s to %s",
			shift.StartTime, shift.EndTime,
			shift.StartDate.Format("2006-01-02"),
			shift.EndDate.Format("2006-01-02")),
		"shift": shift.ToResponse(),
	}
	msg, _ := json.Marshal(payload)

	// Send only to the assigned user
	s.wsHub.SendToUsers([]string{user.ID.String()}, msg)
}

func (s *shiftService) notifyShiftUpdated(shift *model.Shift, originalUserID uuid.UUID, originalUser *model.User) {
	// Check if user was changed (shift reassigned)
	if shift.UserID != originalUserID {
		// Notify OLD user: "Your shift has been reassigned"
		oldPayload := map[string]interface{}{
			"type":   "shift_notification",
			"action": "shift_reassigned_from",
			"message": fmt.Sprintf("Your shift (%s - %s, %s to %s) has been reassigned to %s",
				shift.StartTime, shift.EndTime,
				shift.StartDate.Format("2006-01-02"),
				shift.EndDate.Format("2006-01-02"),
				shift.User.FullName),
			"new_assignee": shift.User.ToResponse(),
		}
		oldMsg, _ := json.Marshal(oldPayload)
		s.wsHub.SendToUsers([]string{originalUserID.String()}, oldMsg)

		// Notify NEW user: "You are replacing X's shift"
		newPayload := map[string]interface{}{
			"type":   "shift_notification",
			"action": "shift_reassigned_to",
			"message": fmt.Sprintf("You are replacing %s's shift: %s - %s, from %s to %s",
				originalUser.FullName,
				shift.StartTime, shift.EndTime,
				shift.StartDate.Format("2006-01-02"),
				shift.EndDate.Format("2006-01-02")),
			"previous_assignee": originalUser.ToResponse(),
			"shift":             shift.ToResponse(),
		}
		newMsg, _ := json.Marshal(newPayload)
		s.wsHub.SendToUsers([]string{shift.UserID.String()}, newMsg)
	} else {
		// Same user, just notify about the update
		payload := map[string]interface{}{
			"type":   "shift_notification",
			"action": "shift_updated",
			"message": fmt.Sprintf("Your shift has been updated: %s - %s, from %s to %s",
				shift.StartTime, shift.EndTime,
				shift.StartDate.Format("2006-01-02"),
				shift.EndDate.Format("2006-01-02")),
			"shift": shift.ToResponse(),
		}
		msg, _ := json.Marshal(payload)
		s.wsHub.SendToUsers([]string{shift.UserID.String()}, msg)
	}
}

func (s *shiftService) notifyShiftDeleted(shift *model.Shift) {
	payload := map[string]interface{}{
		"type":   "shift_notification",
		"action": "shift_cancelled",
		"message": fmt.Sprintf("Your shift has been cancelled: %s - %s, from %s to %s",
			shift.StartTime, shift.EndTime,
			shift.StartDate.Format("2006-01-02"),
			shift.EndDate.Format("2006-01-02")),
		"shift": shift.ToResponse(),
	}
	msg, _ := json.Marshal(payload)

	s.wsHub.SendToUsers([]string{shift.UserID.String()}, msg)
}
