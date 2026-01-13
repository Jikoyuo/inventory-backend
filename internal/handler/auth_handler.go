package handler

import (
	"go-inventory-ws/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ResetPasswordRequest represents the reset password request body
type ResetPasswordRequest struct {
	Email       string `json:"email"`
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// Login handles user authentication
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	if req.Email == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email and password are required"})
	}

	response, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		// Return 401 for authentication errors
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(response)
}

// ResetPassword handles password change
// POST /api/v1/auth/reset-password
func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	if req.Email == "" || req.OldPassword == "" || req.NewPassword == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email, old_password, and new_password are required"})
	}

	if len(req.NewPassword) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": "New password must be at least 6 characters"})
	}

	if err := h.authService.ResetPassword(req.Email, req.OldPassword, req.NewPassword); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Password updated successfully"})
}

func (h *AuthHandler) Heartbeat(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	id, err := uuid.Parse(userID.(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	if err := h.authService.Heartbeat(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update heartbeat"})
	}

	return c.JSON(fiber.Map{"message": "Heartbeat received", "status": "online"})
}

// ValidateTokenRequest represents the validate token request body
type ValidateTokenRequest struct {
	Token string `json:"token"`
}

// ValidateToken handles JWT token validation
// POST /api/v1/auth/validate-token
func (h *AuthHandler) ValidateToken(c *fiber.Ctx) error {
	var req ValidateTokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	if req.Token == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Token is required"})
	}

	response, err := h.authService.ValidateToken(req.Token)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(response)
}
