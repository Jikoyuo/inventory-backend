package handler

import (
	"time"

	"go-inventory-ws/internal/model"
	"go-inventory-ws/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type InventoryHandler struct {
	service service.InventoryService
}

func NewInventoryHandler(s service.InventoryService) *InventoryHandler {
	return &InventoryHandler{service: s}
}

// Helper untuk ambil User Info dari JWT Context (set by auth middleware)
func getUserID(c *fiber.Ctx) string {
	// Extract from JWT context (set by RequireAuth middleware)
	userID := c.Locals("user_id")
	if userID == nil {
		return "system" // Fallback jika tidak ada (shouldn't happen in protected routes)
	}
	return userID.(string)
}

func getUserName(c *fiber.Ctx) string {
	userName := c.Locals("user_name")
	if userName == nil {
		return "Unknown"
	}
	return userName.(string)
}

func getUserEmail(c *fiber.Ctx) string {
	userEmail := c.Locals("user_email")
	if userEmail == nil {
		return ""
	}
	return userEmail.(string)
}

// Helper untuk parse UUID dari string
func parseUUID(id string) (uuid.UUID, error) {
	return uuid.Parse(id)
}

func (h *InventoryHandler) CreateProduct(c *fiber.Ctx) error {
	var product model.Product
	if err := c.BodyParser(&product); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	userID := getUserID(c)
	userName := getUserName(c)
	userEmail := getUserEmail(c)

	if err := h.service.CreateProduct(&product, userID, userName, userEmail); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"message": "Product created", "data": product})
}

func (h *InventoryHandler) CreateTransaction(c *fiber.Ctx) error {
	var tx model.Transaction
	if err := c.BodyParser(&tx); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	userID := getUserID(c)
	userName := getUserName(c)
	userEmail := getUserEmail(c)

	if err := h.service.RecordTransaction(&tx, userID, userName, userEmail); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"message": "Transaction recorded"})
}

func (h *InventoryHandler) UpdateProduct(c *fiber.Ctx) error {
	id := c.Params("id")

	var product model.Product
	if err := c.BodyParser(&product); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	userID := getUserID(c)
	userName := getUserName(c)
	userEmail := getUserEmail(c)

	// Parse UUID
	productID, err := parseUUID(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid product ID"})
	}

	updated, err := h.service.UpdateProduct(productID, &product, userID, userName, userEmail)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Product updated", "data": updated})
}

func (h *InventoryHandler) GetProducts(c *fiber.Ctx) error {
	products, err := h.service.GetAllProducts()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Internal Server Error"})
	}
	return c.JSON(products)
}

func (h *InventoryHandler) GetTransactions(c *fiber.Ctx) error {
	transactions, err := h.service.GetAllTransactions()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Internal Server Error"})
	}
	return c.JSON(transactions)
}

func (h *InventoryHandler) GetTransaction(c *fiber.Ctx) error {
	id := c.Params("id")
	txID, err := parseUUID(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid transaction ID"})
	}

	tx, err := h.service.GetTransactionByID(txID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Transaction not found"})
	}
	return c.JSON(tx)
}

func (h *InventoryHandler) GetFinancialStats(c *fiber.Ctx) error {
	rangeParam := c.Query("range", "7d") // Default 7 days
	now := time.Now()
	var startDate time.Time
	endDate := now

	switch rangeParam {
	case "7d":
		startDate = now.AddDate(0, 0, -7)
	case "1m":
		startDate = now.AddDate(0, -1, 0)
	case "3m":
		startDate = now.AddDate(0, -3, 0)
	case "6m":
		startDate = now.AddDate(0, -6, 0)
	case "12m":
		startDate = now.AddDate(0, -12, 0)
	default:
		// Attempt to parse custom dates? For now fallback to 7d
		startDate = now.AddDate(0, 0, -7)
	}

	stats, err := h.service.GetFinancialStats(startDate, endDate)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}
