package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go-inventory-ws/internal/model"
	"go-inventory-ws/internal/repository"
	"go-inventory-ws/internal/ws"
	"go-inventory-ws/pkg/validator"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InventoryService interface {
	CreateProduct(req *model.Product, userID, userName, userEmail string) error
	UpdateProduct(id uuid.UUID, req *model.Product, userID, userName, userEmail string) (*model.Product, error)
	RecordTransaction(req *model.Transaction, userID, userName, userEmail string) error
	GetAllProducts() ([]model.Product, error)
	GetAllTransactions() ([]model.Transaction, error)
	GetTransactionByID(id uuid.UUID) (*model.Transaction, error)
	GetFinancialStats(startDate, endDate time.Time) (map[string]interface{}, error) // Added
}

type inventoryService struct {
	productRepo     repository.ProductRepository
	transactionRepo repository.TransactionRepository // Added
	db              *gorm.DB
	wsHub           *ws.Hub
}

func NewInventoryService(pRepo repository.ProductRepository, tRepo repository.TransactionRepository, db *gorm.DB, hub *ws.Hub) InventoryService {
	return &inventoryService{
		productRepo:     pRepo,
		transactionRepo: tRepo, // Added
		db:              db,
		wsHub:           hub,
	}
}

func (s *inventoryService) CreateProduct(req *model.Product, userID, userName, userEmail string) error {
	// 1. Validasi Struct Dasar
	if errs := validator.ValidateStruct(req); len(errs) > 0 {
		firstErr := errs[0]
		errorMsg := fmt.Sprintf("Validation failed: Field '%s' failed on tag '%s'", firstErr.FailedField, firstErr.Tag)
		fmt.Println(">>> DEBUG VALIDATION ERROR:", errorMsg)
		return errors.New(errorMsg)
	}

	// 2. Cek Duplikasi SKU (Business Logic Validation)
	existing, _ := s.productRepo.FindBySKU(req.SKU)
	if existing != nil && existing.ID != uuid.Nil {
		return errors.New("SKU already exists")
	}

	// 3. Set Audit Fields and User IDs
	req.CreatedBy = userID
	req.UpdatedBy = userID
	req.CreatedByUserID = &userID
	req.UpdatedByUserID = &userID

	// 4. Simpan ke Database
	if err := s.productRepo.Create(req); err != nil {
		return err
	}

	// 5. Broadcast ke WebSocket dengan user info
	go func() {
		payload := map[string]interface{}{
			"type":   "stock_update",
			"action": "product_created",
			"product": map[string]interface{}{
				"id":    req.ID,
				"sku":   req.SKU,
				"name":  req.Name,
				"stock": req.Stock,
				"price": req.Price,
			},
			"user": map[string]interface{}{
				"id":    userID,
				"name":  userName,
				"email": userEmail,
			},
			"message": fmt.Sprintf("%s created product '%s'", userName, req.Name),
		}
		msg, _ := json.Marshal(payload)
		s.wsHub.Broadcast <- msg
	}()

	return nil
}

func (s *inventoryService) UpdateProduct(id uuid.UUID, req *model.Product, userID, userName, userEmail string) (*model.Product, error) {
	var updatedProduct *model.Product

	// Gunakan Transaction Block dengan Locking
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.Product
		// 1. Cari & Lock Product (Pessimistic Locking)
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&existing, "id = ?", id).Error; err != nil {
			return errors.New("product not found")
		}

		// 2. Track perubahan stock untuk broadcast
		oldStock := existing.Stock

		// 3. Update fields
		existing.Name = req.Name
		existing.SKU = req.SKU
		existing.Stock = req.Stock
		existing.Unit = req.Unit
		existing.Price = req.Price
		existing.UpdatedBy = userID
		existing.UpdatedByUserID = &userID

		// 4. Simpan ke database (pakai tx)
		if err := s.productRepo.UpdateStock(tx, existing.ID, existing.Stock, userID); err != nil {
			// Note: productRepo.Update mungkin tidak support tx arg standard,
			// sebaiknya kita pakai tx.Save langsung disini untuk konsistensi locking
			return tx.Save(&existing).Error
		}

		updatedProduct = &existing

		// 5. Broadcast ke WebSocket (move inside success path but outside tx usually,
		// to avoid broadcast on rollback. But we use goroutine so it's fine)
		go func() {
			payload := map[string]interface{}{
				"type":   "stock_update",
				"action": "product_updated",
				"product": map[string]interface{}{
					"id":        existing.ID,
					"sku":       existing.SKU,
					"name":      existing.Name,
					"old_stock": oldStock,
					"new_stock": existing.Stock,
					"price":     existing.Price,
				},
				"user": map[string]interface{}{
					"id":    userID,
					"name":  userName,
					"email": userEmail,
				},
				"message": fmt.Sprintf("%s updated product '%s'", userName, existing.Name),
			}
			msg, _ := json.Marshal(payload)
			s.wsHub.Broadcast <- msg
		}()

		return nil
	})

	if err != nil {
		return nil, err
	}

	return updatedProduct, nil
}

func (s *inventoryService) RecordTransaction(req *model.Transaction, userID, userName, userEmail string) error {
	// 1. Validasi Input
	if errs := validator.ValidateStruct(req); len(errs) > 0 {
		firstErr := errs[0]
		errorMsg := fmt.Sprintf("Validation failed: Field '%s' failed on tag '%s'", firstErr.FailedField, firstErr.Tag)
		fmt.Println(">>> DEBUG TRANSACTION VALIDATION ERROR:", errorMsg)
		return errors.New(errorMsg)
	}

	// 1b. Strict Validation for Payment and Logic
	// User requested "bisa diisi 0" (can be 0) for now until payment gateway is setup.
	if req.PaymentMethod != "CASH" && req.PaymentMethod != "TRANSFER" && req.PaymentMethod != "0" && req.PaymentMethod != "" {
		return errors.New("invalid payment method: must be CASH, TRANSFER, or 0")
	}

	// Gunakan Transaction Block (Atomic Operation)
	return s.db.Transaction(func(tx *gorm.DB) error {
		var product model.Product
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&product, "id = ?", req.ProductID).Error; err != nil {
			return errors.New("product not found")
		}

		// Calculate Total Amount accurately (Snapshot)
		req.TotalAmount = product.Price * int64(req.Quantity)

		// B. Hitung Logic Stok
		newStock := product.Stock
		if req.Type == model.TxIn {
			newStock += req.Quantity
		} else if req.Type == model.TxOut {
			if product.Stock < req.Quantity {
				return errors.New("insufficient stock remaining")
			}
			newStock -= req.Quantity
		}

		// C. Update Stok Product
		if err := s.productRepo.UpdateStock(tx, product.ID, newStock, userID); err != nil {
			return err
		}

		// D. Simpan Log Transaksi dengan user ID
		req.CreatedBy = userID
		req.UpdatedBy = userID
		req.CreatedByUserID = &userID
		if err := tx.Create(req).Error; err != nil {
			return err
		}

		// E. Broadcast ke WebSocket dengan user info
		go func() {
			actionType := "IN"
			actionVerb := "added"
			if req.Type == model.TxOut {
				actionType = "OUT"
				actionVerb = "removed"
			}

			// Broadcast Stock Update
			stockPayload := map[string]interface{}{
				"type":   "stock_update",
				"action": "transaction_created",
				"transaction": map[string]interface{}{
					"id":             req.ID,
					"type":           actionType,
					"quantity":       req.Quantity,
					"total_amount":   req.TotalAmount,
					"payment_method": req.PaymentMethod,
					"product_id":     product.ID,
					"product": map[string]interface{}{
						"name": product.Name,
						"sku":  product.SKU,
					},
					"new_stock": newStock,
				},
				"user": map[string]interface{}{
					"id":    userID,
					"name":  userName,
					"email": userEmail,
				},
				"message": fmt.Sprintf("%s %s %d units of '%s' (%s)", userName, actionVerb, req.Quantity, product.Name, actionType),
			}
			stockMsg, _ := json.Marshal(stockPayload)
			s.wsHub.Broadcast <- stockMsg

			// Broadcast Financial Update (Notify that financial stats might have changed)
			// Clients should re-fetch /api/finance/stats or we can push a flag
			finPayload := map[string]interface{}{
				"type":    "financial_update",
				"message": "Financial stats updated due to new transaction",
			}
			finMsg, _ := json.Marshal(finPayload)
			s.wsHub.Broadcast <- finMsg
		}()

		return nil
	})
}

func (s *inventoryService) GetAllProducts() ([]model.Product, error) {
	return s.productRepo.FindAll()
}

func (s *inventoryService) GetAllTransactions() ([]model.Transaction, error) {
	return s.transactionRepo.FindAll()
}

func (s *inventoryService) GetTransactionByID(id uuid.UUID) (*model.Transaction, error) {
	return s.transactionRepo.FindByID(id)
}

func (s *inventoryService) GetFinancialStats(startDate, endDate time.Time) (map[string]interface{}, error) {
	// 1. Get Income and Expense
	income, expense, err := s.transactionRepo.GetFinancialSummary(startDate, endDate)
	if err != nil {
		return nil, err
	}

	// 2. Get Current Valuation and other stats (using existing Dashboard logic or part of it)
	// Usually dashboard stats are overall, but Valuation is snapshot.
	// We can reuse GetDashboardStats for valuation.
	stats, err := s.transactionRepo.GetDashboardStats()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_income":    income,
		"total_expense":   expense,
		"total_valuation": stats.TotalValuation, // Current snapshot
		"period_start":    startDate.Format("2006-01-02"),
		"period_end":      endDate.Format("2006-01-02"),
	}, nil
}
