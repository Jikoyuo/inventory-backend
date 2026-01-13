package repository

import (
	"go-inventory-ws/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProductRepository interface {
	Create(product *model.Product) error
	FindAll() ([]model.Product, error)
	FindByID(id uuid.UUID) (*model.Product, error)
	FindBySKU(sku string) (*model.Product, error)
	Update(product *model.Product) error
	UpdateStock(tx *gorm.DB, id uuid.UUID, newStock int, updatedBy string) error
}

type productRepo struct {
	db *gorm.DB
}

func NewProductRepo(db *gorm.DB) ProductRepository {
	return &productRepo{db}
}

func (r *productRepo) Create(product *model.Product) error {
	return r.db.Create(product).Error
}

func (r *productRepo) FindAll() ([]model.Product, error) {
	var products []model.Product
	err := r.db.Preload("CreatedByUser").Preload("UpdatedByUser").Find(&products).Error
	return products, err
}

func (r *productRepo) FindByID(id uuid.UUID) (*model.Product, error) {
	var product model.Product
	err := r.db.Preload("CreatedByUser").Preload("UpdatedByUser").First(&product, "id = ?", id).Error
	return &product, err
}

func (r *productRepo) FindBySKU(sku string) (*model.Product, error) {
	var product model.Product
	err := r.db.First(&product, "sku = ?", sku).Error
	return &product, err
}

func (r *productRepo) Update(product *model.Product) error {
	return r.db.Save(product).Error
}

// UpdateStock menerima *gorm.DB (tx) agar bisa berjalan dalam transaksi
func (r *productRepo) UpdateStock(tx *gorm.DB, id uuid.UUID, newStock int, updatedBy string) error {
	return tx.Model(&model.Product{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"stock":      newStock,
			"updated_by": updatedBy,
		}).Error
}
