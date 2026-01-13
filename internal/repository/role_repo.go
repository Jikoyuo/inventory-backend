package repository

import (
	"go-inventory-ws/internal/model"

	"gorm.io/gorm"
)

type RoleRepository interface {
	FindAll() ([]model.Role, error)
	FindByID(id uint) (*model.Role, error)
	FindByCode(code string) (*model.Role, error)
	Create(role *model.Role) error
	SeedDefaults() error
}

type roleRepo struct {
	db *gorm.DB
}

func NewRoleRepo(db *gorm.DB) RoleRepository {
	return &roleRepo{db: db}
}

func (r *roleRepo) FindAll() ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Preload("Privileges").Find(&roles).Error
	return roles, err
}

func (r *roleRepo) FindByID(id uint) (*model.Role, error) {
	var role model.Role
	err := r.db.Preload("Privileges").First(&role, id).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepo) FindByCode(code string) (*model.Role, error) {
	var role model.Role
	err := r.db.Preload("Privileges").Where("code = ?", code).First(&role).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepo) Create(role *model.Role) error {
	return r.db.Create(role).Error
}

func (r *roleRepo) SeedDefaults() error {
	for _, defaultRole := range model.DefaultRoles {
		var existingRole model.Role
		err := r.db.Where("code = ?", defaultRole.Code).First(&existingRole).Error
		if err == gorm.ErrRecordNotFound {
			// Role doesn't exist, create it
			if err := r.db.Create(&defaultRole).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
