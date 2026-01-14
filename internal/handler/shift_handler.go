package handler

import (
	"time"

	"go-inventory-ws/internal/model"
	"go-inventory-ws/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ShiftHandler struct {
	shiftService service.ShiftService
}

func NewShiftHandler(shiftService service.ShiftService) *ShiftHandler {
	return &ShiftHandler{shiftService: shiftService}
}

// CreateShift handles shift creation
// POST /api/v1/shifts
// Only MASTER_ADMIN can create shifts (enforced by middleware)
func (h *ShiftHandler) CreateShift(c *fiber.Ctx) error {
	var req service.CreateShiftRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	// Get creator ID from context (set by auth middleware)
	creatorID := c.Locals("user_id")
	if creatorID == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	shift, err := h.shiftService.CreateShift(&req, creatorID.(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Shift created successfully",
		"data":    shift.ToResponse(),
	})
}

// UpdateShift handles shift update
// PUT /api/v1/shifts/:id
// Only MASTER_ADMIN can update shifts (enforced by middleware)
func (h *ShiftHandler) UpdateShift(c *fiber.Ctx) error {
	id := c.Params("id")
	shiftID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid shift ID"})
	}

	var req service.UpdateShiftRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	// Get updater ID from context
	updaterID := c.Locals("user_id")
	if updaterID == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	shift, err := h.shiftService.UpdateShift(shiftID, &req, updaterID.(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "Shift updated successfully",
		"data":    shift.ToResponse(),
	})
}

// DeleteShift handles shift deletion
// DELETE /api/v1/shifts/:id
// Only MASTER_ADMIN can delete shifts (enforced by middleware)
func (h *ShiftHandler) DeleteShift(c *fiber.Ctx) error {
	id := c.Params("id")
	shiftID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid shift ID"})
	}

	// Get deleter ID from context
	deleterID := c.Locals("user_id")
	if deleterID == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	if err := h.shiftService.DeleteShift(shiftID, deleterID.(string)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Shift deleted successfully"})
}

// GetShift handles getting a single shift by ID
// GET /api/v1/shifts/:id
// MASTER_ADMIN can view any shift, other users can only view their own
func (h *ShiftHandler) GetShift(c *fiber.Ctx) error {
	id := c.Params("id")
	shiftID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid shift ID"})
	}

	// Get requester info from context
	requesterID := c.Locals("user_id")
	if requesterID == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Check if requester is MASTER_ADMIN
	isMasterAdmin := h.checkMasterAdmin(c)

	shift, err := h.shiftService.GetShiftByID(shiftID, requesterID.(string), isMasterAdmin)
	if err != nil {
		if err == service.ErrUnauthorizedShiftView {
			return c.Status(403).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": shift})
}

// GetShifts handles getting shifts with view type filter
// GET /api/v1/shifts
// Query params: view_type (daily/weekly/monthly/all), reference_date (YYYY-MM-DD)
// MASTER_ADMIN sees all shifts, other users see only their own
func (h *ShiftHandler) GetShifts(c *fiber.Ctx) error {
	// Get requester info from context
	requesterID := c.Locals("user_id")
	if requesterID == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Check if requester is MASTER_ADMIN
	isMasterAdmin := h.checkMasterAdmin(c)

	// Get view type from query (default: all)
	viewType := c.Query("view_type", string(model.ViewTypeAll))

	// Get reference date from query (default: today)
	referenceDateStr := c.Query("reference_date", "")
	var referenceDate time.Time
	if referenceDateStr != "" {
		parsed, err := time.Parse("2006-01-02", referenceDateStr)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid reference_date format, use YYYY-MM-DD"})
		}
		referenceDate = parsed
	} else {
		referenceDate = time.Now()
	}

	shifts, err := h.shiftService.GetShifts(requesterID.(string), isMasterAdmin, viewType, referenceDate)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":            shifts,
		"view_type":       viewType,
		"reference_date":  referenceDate.Format("2006-01-02"),
		"is_master_admin": isMasterAdmin,
		"total":           len(shifts),
	})
}

// GetShiftsByUser handles getting shifts for a specific user
// GET /api/v1/shifts/user/:user_id
// MASTER_ADMIN can view any user's shifts, other users can only view their own
func (h *ShiftHandler) GetShiftsByUser(c *fiber.Ctx) error {
	id := c.Params("user_id")
	userID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	// Get requester info from context
	requesterID := c.Locals("user_id")
	if requesterID == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Check if requester is MASTER_ADMIN
	isMasterAdmin := h.checkMasterAdmin(c)

	shifts, err := h.shiftService.GetShiftsByUser(userID, requesterID.(string), isMasterAdmin)
	if err != nil {
		if err == service.ErrUnauthorizedShiftView {
			return c.Status(403).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":    shifts,
		"user_id": userID.String(),
		"total":   len(shifts),
	})
}

// checkMasterAdmin checks if the requester has MASTER_ADMIN role
// by checking their privileges
func (h *ShiftHandler) checkMasterAdmin(c *fiber.Ctx) bool {
	privileges, ok := c.Locals("user_privileges").([]string)
	if !ok {
		return false
	}

	// MASTER_ADMIN has shift:create privilege (among others)
	// We check for a MASTER_ADMIN-only privilege
	for _, p := range privileges {
		if p == "shift:create" || p == "user:create" {
			return true
		}
	}

	return false
}
