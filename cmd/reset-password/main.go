package main

import (
	"log"

	"go-inventory-ws/internal/model"
	"go-inventory-ws/pkg/database"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// 1. Load Env
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, replying on system env")
	}

	// 2. Setup Database
	db := database.ConnectDB()

	// 3. Find Admin
	email := "admin@example.com"
	var user model.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		log.Fatalf("❌ User %s not found in database: %v", email, err)
	}

	// 4. Hash new password
	newPassword := "admin123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("❌ Failed to hash password: %v", err)
	}

	// 5. Update
	if err := db.Model(&user).Update("password", string(hashedPassword)).Error; err != nil {
		log.Fatalf("❌ Failed to update password in DB: %v", err)
	}

	log.Printf("✅ Success! Password for %s has been reset to: %s", email, newPassword)
	log.Printf("Current Hash in DB: %s", string(hashedPassword))
}
