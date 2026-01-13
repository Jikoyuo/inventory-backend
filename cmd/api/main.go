package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"go-inventory-ws/internal/handler"
	"go-inventory-ws/internal/middleware"
	"go-inventory-ws/internal/model"
	"go-inventory-ws/internal/repository"
	"go-inventory-ws/internal/service"
	"go-inventory-ws/internal/ws"
	"go-inventory-ws/pkg/database"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	// 1. Load Env
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	// 2. Setup Database
	db := database.ConnectDB()
	// Auto Migrate (Hati-hati di production, sebaiknya pakai tools migrasi terpisah)
	db.AutoMigrate(&model.Product{}, &model.Transaction{}, &model.User{}, &model.Privilege{}, &model.Role{})

	// 3. Seed default privileges, roles, and admin user
	seedPrivilegesRolesAndAdmin(db)

	// 4. Setup WebSocket Hub
	wsHub := ws.NewHub()
	go wsHub.Run()

	// 5. Dependency Injection (Wiring Layers)
	productRepo := repository.NewProductRepo(db)
	txRepo := repository.NewTransactionRepo(db)
	userRepo := repository.NewUserRepo(db)
	privilegeRepo := repository.NewPrivilegeRepo(db)
	roleRepo := repository.NewRoleRepo(db)

	invService := service.NewInventoryService(productRepo, txRepo, db, wsHub)
	dashService := service.NewDashboardService(txRepo)
	authService := service.NewAuthService(userRepo, wsHub)
	userService := service.NewUserService(userRepo, privilegeRepo, roleRepo)

	invHandler := handler.NewInventoryHandler(invService)
	dashHandler := handler.NewDashboardHandler(dashService)
	authHandler := handler.NewAuthHandler(authService)
	userHandler := handler.NewUserHandler(userService)
	roleHandler := handler.NewRoleHandler(roleRepo)

	// 6. Setup Fiber
	app := fiber.New(fiber.Config{
		AppName: "Inventory General Pro v1.0",
	})

	// Middleware
	app.Use(logger.New())  // Logging request
	app.Use(recover.New()) // Panic recovery
	app.Use(cors.New())    // CORS

	// 7. Routes
	api := app.Group("/api/v1")

	// ============ PUBLIC ROUTES ============
	// Auth Routes (No authentication required)
	auth := api.Group("/auth")
	auth.Post("/login", authHandler.Login)
	auth.Post("/reset-password", authHandler.ResetPassword)
	auth.Post("/validate-token", authHandler.ValidateToken)
	auth.Post("/heartbeat", middleware.RequireAuth(userRepo), authHandler.Heartbeat) // Heartbeat uses Auth but available to all authenticated

	// ============ PROTECTED ROUTES ============
	// All routes below require authentication
	protected := api.Group("", middleware.RequireAuth(userRepo))

	// Dashboard Routes (authenticated users can view)
	protected.Get("/dashboard/stats", dashHandler.GetDashboardStats)
	protected.Get("/dashboard/stock-movement", dashHandler.GetStockMovement)

	// Product Routes (with privilege checks)
	protected.Get("/products", invHandler.GetProducts)
	protected.Post("/products", middleware.RequirePrivilege("product:create"), invHandler.CreateProduct)
	protected.Put("/products/:id", middleware.RequirePrivilege("product:update"), invHandler.UpdateProduct)

	// Transaction Routes (with privilege checks)
	protected.Get("/transactions", middleware.RequirePrivilege("transaction:view"), invHandler.GetTransactions)
	protected.Get("/transactions/:id", middleware.RequirePrivilege("transaction:view"), invHandler.GetTransaction)
	protected.Post("/transactions", middleware.RequirePrivilege("transaction:create"), invHandler.CreateTransaction)

	// User Management Routes (with privilege checks)
	protected.Get("/users", userHandler.GetUsers)
	protected.Get("/users/:id", userHandler.GetUser)
	protected.Post("/users", middleware.RequirePrivilege("user:create"), userHandler.CreateUser)
	protected.Put("/users/:id", middleware.RequirePrivilege("user:update"), userHandler.UpdateUser)
	protected.Delete("/users/:id", middleware.RequirePrivilege("user:delete"), userHandler.DeleteUser)
	protected.Put("/users/:id/privileges", middleware.RequirePrivilege("user:update_privilege"), userHandler.UpdateUserPrivileges)

	// Role Routes
	protected.Get("/roles", roleHandler.GetRoles)

	// Privileges Route (list all available privileges)
	protected.Get("/privileges", func(c *fiber.Ctx) error {
		privileges, err := privilegeRepo.FindAll()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch privileges"})
		}
		return c.JSON(privileges)
	})

	// WebSocket Route
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return c.SendStatus(fiber.StatusUpgradeRequired)
	})
	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		wsHub.Register <- c
		defer func() { wsHub.Unregister <- c }()

		for {
			// Keep alive loop
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
	}))

	// 8. Graceful Shutdown
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "3000"
		}
		if err := app.Listen(":" + port); err != nil {
			log.Panic(err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	if err := app.Shutdown(); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}

// seedPrivilegesRolesAndAdmin creates default privileges, roles, and admin user if they don't exist
func seedPrivilegesRolesAndAdmin(db *gorm.DB) {
	privilegeRepo := repository.NewPrivilegeRepo(db)
	userRepo := repository.NewUserRepo(db)
	roleRepo := repository.NewRoleRepo(db)

	// 1. Seed privileges first
	if err := privilegeRepo.SeedDefaults(); err != nil {
		log.Printf("Warning: Failed to seed privileges: %v", err)
	}

	// 2. Seed roles
	if err := roleRepo.SeedDefaults(); err != nil {
		log.Printf("Warning: Failed to seed roles: %v", err)
	}

	// 3. Assign privileges to roles
	allPrivileges, _ := privilegeRepo.FindAll()

	// MASTER_ADMIN gets ALL privileges
	masterRole, err := roleRepo.FindByCode(model.RoleMasterAdmin)
	if err == nil && len(masterRole.Privileges) == 0 {
		db.Model(&masterRole).Association("Privileges").Replace(allPrivileges)
		log.Println("✅ MASTER_ADMIN role assigned all privileges")
	}

	// ADMIN gets limited privileges (exclude user management)
	adminRole, err := roleRepo.FindByCode(model.RoleAdmin)
	if err == nil && len(adminRole.Privileges) == 0 {
		adminPrivileges := []model.Privilege{}
		for _, p := range allPrivileges {
			// Exclude user creation, update, delete, and privilege update
			if p.Code != "user:create" && p.Code != "user:update" && p.Code != "user:delete" && p.Code != "user:update_privilege" {
				adminPrivileges = append(adminPrivileges, p)
			}
		}
		db.Model(&adminRole).Association("Privileges").Replace(adminPrivileges)
		log.Println("✅ ADMIN role assigned limited privileges")
	}

	// 4. Create default admin user with MASTER_ADMIN role
	_, err = userRepo.FindByEmail("admin@example.com")
	if err != nil {
		// Create admin user
		masterRole, _ := roleRepo.FindByCode(model.RoleMasterAdmin)

		admin := &model.User{
			Email:       "admin@example.com",
			FullName:    "Master Administrator",
			PhoneNumber: "",
			RoleID:      &masterRole.ID,
			IsActive:    true,
			Privileges:  masterRole.Privileges,
		}
		admin.CreatedBy = "system"
		admin.UpdatedBy = "system"

		if err := admin.SetPassword("admin123"); err != nil {
			log.Printf("Warning: Failed to hash admin password: %v", err)
			return
		}

		if err := userRepo.Create(admin); err != nil {
			log.Printf("Warning: Failed to create admin user: %v", err)
		} else {
			log.Println("✅ Admin user created: admin@example.com / admin123 (MASTER_ADMIN)")
		}
	}
}
