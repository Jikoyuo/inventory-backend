package repository

import (
	"time"

	"go-inventory-ws/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransactionRepository interface {
	GetStockMovement(startDate, endDate time.Time) ([]StockMovementData, error)
	GetDashboardStats() (*DashboardStats, error)
	FindAll() ([]model.Transaction, error)
	FindByID(id uuid.UUID) (*model.Transaction, error)
}

// StockMovementData untuk chart data
type StockMovementData struct {
	Date     string `json:"date"`
	Inbound  int    `json:"inbound"`
	Outbound int    `json:"outbound"`
}

// DashboardStats untuk overview stats
type DashboardStats struct {
	TotalProducts  int64 `json:"total_products"`
	LowStockCount  int64 `json:"low_stock_count"`
	TotalValuation int64 `json:"total_valuation"`
}

type transactionRepo struct {
	db *gorm.DB
}

func NewTransactionRepo(db *gorm.DB) TransactionRepository {
	return &transactionRepo{db}
}

func (r *transactionRepo) GetStockMovement(startDate, endDate time.Time) ([]StockMovementData, error) {
	var results []StockMovementData

	// Query untuk aggregate transactions per hari
	rows, err := r.db.Model(&model.Transaction{}).
		Select(`
			DATE(created_at) as date,
			COALESCE(SUM(CASE WHEN type = 'IN' THEN quantity ELSE 0 END), 0) as inbound,
			COALESCE(SUM(CASE WHEN type = 'OUT' THEN quantity ELSE 0 END), 0) as outbound
		`).
		Where("created_at BETWEEN ? AND ?", startDate, endDate).
		Group("DATE(created_at)").
		Order("date ASC").
		Rows()

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var data StockMovementData
		if err := rows.Scan(&data.Date, &data.Inbound, &data.Outbound); err != nil {
			return nil, err
		}
		results = append(results, data)
	}

	return results, nil
}

func (r *transactionRepo) FindAll() ([]model.Transaction, error) {
	var transactions []model.Transaction
	// Preload Product dan CreatedByUser
	err := r.db.Preload("Product").Preload("CreatedByUser").Order("created_at DESC").Find(&transactions).Error
	return transactions, err
}

func (r *transactionRepo) FindByID(id uuid.UUID) (*model.Transaction, error) {
	var transaction model.Transaction
	err := r.db.Preload("Product").Preload("CreatedByUser").First(&transaction, "id = ?", id).Error
	return &transaction, err
}

func (r *transactionRepo) GetDashboardStats() (*DashboardStats, error) {
	var stats DashboardStats

	// Total Products
	r.db.Model(&model.Product{}).Count(&stats.TotalProducts)

	// Low Stock Count (stock < 10)
	r.db.Model(&model.Product{}).Where("stock < ?", 10).Count(&stats.LowStockCount)

	// Total Valuation (SUM of stock * price)
	r.db.Model(&model.Product{}).Select("COALESCE(SUM(stock * price), 0)").Scan(&stats.TotalValuation)

	return &stats, nil
}
