package service

import (
	"encoding/json"
	"errors"
	"fmt"

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

	// Gunakan Transaction Block (Atomic Operation)
	return s.db.Transaction(func(tx *gorm.DB) error {
		var product model.Product
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&product, "id = ?", req.ProductID).Error; err != nil {
			return errors.New("product not found")
		}

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

			payload := map[string]interface{}{
				"type":   "stock_update",
				"action": "transaction_created",
				"transaction": map[string]interface{}{
					"id":         req.ID,
					"type":       actionType,
					"quantity":   req.Quantity,
					"product_id": product.ID,
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
			msg, _ := json.Marshal(payload)
			s.wsHub.Broadcast <- msg
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
