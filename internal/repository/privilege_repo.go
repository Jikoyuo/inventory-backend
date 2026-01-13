package repository

import (
	"go-inventory-ws/internal/model"

	"gorm.io/gorm"
)

type PrivilegeRepository interface {
	FindByCode(code string) (*model.Privilege, error)
	FindByCodes(codes []string) ([]model.Privilege, error)
	FindAll() ([]model.Privilege, error)
	Create(privilege *model.Privilege) error
	SeedDefaults() error
}

type privilegeRepo struct {
	db *gorm.DB
}

func NewPrivilegeRepo(db *gorm.DB) PrivilegeRepository {
	return &privilegeRepo{db}
}

func (r *privilegeRepo) FindByCode(code string) (*model.Privilege, error) {
	var privilege model.Privilege
	if err := r.db.Where("code = ?", code).First(&privilege).Error; err != nil {
		return nil, err
	}
	return &privilege, nil
}

func (r *privilegeRepo) FindByCodes(codes []string) ([]model.Privilege, error) {
	var privileges []model.Privilege
	if err := r.db.Where("code IN ?", codes).Find(&privileges).Error; err != nil {
		return nil, err
	}
	return privileges, nil
}

func (r *privilegeRepo) FindAll() ([]model.Privilege, error) {
	var privileges []model.Privilege
	if err := r.db.Find(&privileges).Error; err != nil {
		return nil, err
	}
	return privileges, nil
}

func (r *privilegeRepo) Create(privilege *model.Privilege) error {
	return r.db.Create(privilege).Error
}

// SeedDefaults creates default privileges if they don't exist
func (r *privilegeRepo) SeedDefaults() error {
	for _, p := range model.DefaultPrivileges {
		var existing model.Privilege
		if err := r.db.Where("code = ?", p.Code).First(&existing).Error; err == gorm.ErrRecordNotFound {
			if err := r.db.Create(&p).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
