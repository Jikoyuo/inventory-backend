package service

import (
	"time"

	"go-inventory-ws/internal/repository"
)

type DashboardService interface {
	GetStockMovement(days int) ([]repository.StockMovementData, error)
	GetDashboardStats() (*repository.DashboardStats, error)
}

type dashboardService struct {
	txRepo repository.TransactionRepository
}

func NewDashboardService(txRepo repository.TransactionRepository) DashboardService {
	return &dashboardService{txRepo: txRepo}
}

func (s *dashboardService) GetStockMovement(days int) ([]repository.StockMovementData, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	return s.txRepo.GetStockMovement(startDate, endDate)
}

func (s *dashboardService) GetDashboardStats() (*repository.DashboardStats, error) {
	return s.txRepo.GetDashboardStats()
}
