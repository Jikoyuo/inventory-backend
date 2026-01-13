package handler

import (
	"strconv"

	"go-inventory-ws/internal/service"

	"github.com/gofiber/fiber/v2"
)

type DashboardHandler struct {
	service service.DashboardService
}

func NewDashboardHandler(s service.DashboardService) *DashboardHandler {
	return &DashboardHandler{service: s}
}

// GetStockMovement returns stock movement data for charts
// Query params: days (default 7)
func (h *DashboardHandler) GetStockMovement(c *fiber.Ctx) error {
	daysStr := c.Query("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 7
	}

	data, err := h.service.GetStockMovement(days)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch stock movement"})
	}

	return c.JSON(fiber.Map{
		"period": days,
		"data":   data,
	})
}

// GetDashboardStats returns overview statistics
func (h *DashboardHandler) GetDashboardStats(c *fiber.Ctx) error {
	stats, err := h.service.GetDashboardStats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch dashboard stats"})
	}

	return c.JSON(stats)
}
