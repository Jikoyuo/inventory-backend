package handler

import (
	"go-inventory-ws/internal/repository"

	"github.com/gofiber/fiber/v2"
)

type RoleHandler struct {
	roleRepo repository.RoleRepository
}

func NewRoleHandler(roleRepo repository.RoleRepository) *RoleHandler {
	return &RoleHandler{roleRepo: roleRepo}
}

// GetRoles returns all available roles
// GET /api/v1/roles
func (h *RoleHandler) GetRoles(c *fiber.Ctx) error {
	roles, err := h.roleRepo.FindAll()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch roles"})
	}
	return c.JSON(roles)
}
