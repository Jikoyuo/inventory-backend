package middleware

import (
	"strings"

	"go-inventory-ws/internal/repository"
	"go-inventory-ws/pkg/jwt"

	"github.com/gofiber/fiber/v2"
)

// RequireAuth is middleware that validates JWT token and sets user info in context
func RequireAuth(userRepo repository.UserRepository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{"error": "Missing authorization token"})
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid authorization format. Use: Bearer <token>"})
		}

		tokenString := parts[1]

		// Validate token
		claims, err := jwt.ValidateToken(tokenString)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid or expired token"})
		}

		// Check strict session against DB
		user, err := userRepo.FindByID(claims.UserID)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "User not found"})
		}

		if user.TokenVersion != claims.TokenVersion {
			return c.Status(401).JSON(fiber.Map{"error": "Session expired (logged in on another device)"})
		}

		// Set user info in context for downstream handlers
		c.Locals("user_id", claims.UserID.String())
		c.Locals("user_email", claims.Email)
		c.Locals("user_name", claims.Name)
		c.Locals("user_privileges", claims.Privileges)

		return c.Next()
	}
}

// RequirePrivilege checks if the authenticated user has the required privilege
func RequirePrivilege(requiredPrivilege string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get privileges from context (set by RequireAuth)
		privileges, ok := c.Locals("user_privileges").([]string)
		if !ok {
			return c.Status(403).JSON(fiber.Map{"error": "No privileges found"})
		}

		// Check if user has the required privilege
		for _, p := range privileges {
			if p == requiredPrivilege {
				return c.Next()
			}
		}

		return c.Status(403).JSON(fiber.Map{
			"error": "Forbidden: requires '" + requiredPrivilege + "' privilege",
		})
	}
}

// RequireAnyPrivilege checks if the user has at least one of the specified privileges
func RequireAnyPrivilege(requiredPrivileges ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		privileges, ok := c.Locals("user_privileges").([]string)
		if !ok {
			return c.Status(403).JSON(fiber.Map{"error": "No privileges found"})
		}

		for _, userPriv := range privileges {
			for _, reqPriv := range requiredPrivileges {
				if userPriv == reqPriv {
					return c.Next()
				}
			}
		}

		return c.Status(403).JSON(fiber.Map{
			"error": "Forbidden: requires one of " + strings.Join(requiredPrivileges, ", ") + " privileges",
		})
	}
}
