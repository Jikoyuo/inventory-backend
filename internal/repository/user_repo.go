package repository

import (
	"go-inventory-ws/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository interface {
	FindByEmail(email string) (*model.User, error)
	FindByID(id uuid.UUID) (*model.User, error)
	Create(user *model.User) error
	Update(user *model.User) error
	Delete(id uuid.UUID) error
	UpdatePassword(userID uuid.UUID, hashedPassword string) error
	UpdatePrivileges(userID uuid.UUID, privileges []model.Privilege) error
	FindAll() ([]model.User, error)
	UpdateTokenVersion(userID uuid.UUID, version string) error
	UpdateLastSeen(userID uuid.UUID) error
}

type userRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) UserRepository {
	return &userRepo{db}
}

func (r *userRepo) FindByEmail(email string) (*model.User, error) {
	var user model.User
	if err := r.db.Preload("Role").Preload("Privileges").Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) FindByID(id uuid.UUID) (*model.User, error) {
	var user model.User
	if err := r.db.Preload("Role").Preload("Privileges").First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) Create(user *model.User) error {
	return r.db.Create(user).Error
}

func (r *userRepo) Update(user *model.User) error {
	return r.db.Save(user).Error
}

func (r *userRepo) UpdatePassword(userID uuid.UUID, hashedPassword string) error {
	return r.db.Model(&model.User{}).Where("id = ?", userID).Update("password", hashedPassword).Error
}

func (r *userRepo) UpdatePrivileges(userID uuid.UUID, privileges []model.Privilege) error {
	var user model.User
	if err := r.db.First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	return r.db.Model(&user).Association("Privileges").Replace(privileges)
}

func (r *userRepo) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.User{}, "id = ?", id).Error
}

func (r *userRepo) FindAll() ([]model.User, error) {
	var users []model.User
	if err := r.db.Preload("Role").Preload("Privileges").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *userRepo) UpdateTokenVersion(userID uuid.UUID, version string) error {
	return r.db.Model(&model.User{}).Where("id = ?", userID).Update("token_version", version).Error
}

func (r *userRepo) UpdateLastSeen(userID uuid.UUID) error {
	return r.db.Model(&model.User{}).Where("id = ?", userID).Update("last_seen_at", gorm.Expr("NOW()")).Error
}
