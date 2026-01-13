package service

import (
	"errors"
	"fmt"
	"time"

	"go-inventory-ws/internal/model"
	"go-inventory-ws/internal/repository"
	"go-inventory-ws/pkg/validator"

	"github.com/google/uuid"
)

var (
	ErrEmailExists = errors.New("email already exists")
)

type UserService interface {
	CreateUser(req *CreateUserRequest, creatorID string) (*model.User, error)
	UpdateUser(userID uuid.UUID, req *UpdateUserRequest, updaterID string) (*model.User, error)
	DeleteUser(userID uuid.UUID) error
	UpdateUserPrivileges(userID uuid.UUID, privilegeCodes []string, updaterID string) (*model.User, error)
	GetAllUsers() ([]model.UserResponse, error)
	GetUserByID(id uuid.UUID) (*model.UserResponse, error)
}

type CreateUserRequest struct {
	Email       string  `json:"email" validate:"required,email"`
	Password    string  `json:"password" validate:"required,min=6"`
	FullName    string  `json:"full_name" validate:"required"`
	PhoneNumber string  `json:"phone_number"`
	BirthDate   *string `json:"birth_date"` // Format: YYYY-MM-DD
	RoleID      uint    `json:"role_id" validate:"required"`
}

type UpdateUserRequest struct {
	Email       string  `json:"email" validate:"required,email"`
	Password    *string `json:"password,omitempty" validate:"omitempty,min=6"` // Optional
	FullName    string  `json:"full_name" validate:"required"`
	PhoneNumber string  `json:"phone_number"`
	BirthDate   *string `json:"birth_date"` // Format: YYYY-MM-DD
	RoleID      uint    `json:"role_id" validate:"required"`
	IsActive    *bool   `json:"is_active"`
}

type userService struct {
	userRepo      repository.UserRepository
	privilegeRepo repository.PrivilegeRepository
	roleRepo      repository.RoleRepository
}

func NewUserService(userRepo repository.UserRepository, privilegeRepo repository.PrivilegeRepository, roleRepo repository.RoleRepository) UserService {
	return &userService{
		userRepo:      userRepo,
		privilegeRepo: privilegeRepo,
		roleRepo:      roleRepo,
	}
}

func (s *userService) CreateUser(req *CreateUserRequest, creatorID string) (*model.User, error) {
	// 1. Validate request
	if errs := validator.ValidateStruct(req); len(errs) > 0 {
		firstErr := errs[0]
		return nil, fmt.Errorf("Validation failed: Field '%s' failed on tag '%s'", firstErr.FailedField, firstErr.Tag)
	}

	// 2. Check if email already exists
	existing, _ := s.userRepo.FindByEmail(req.Email)
	if existing != nil {
		return nil, ErrEmailExists
	}

	// 3. Validate role exists
	role, err := s.roleRepo.FindByID(req.RoleID)
	if err != nil {
		return nil, errors.New("role not found")
	}

	// 4. Parse birthdate if provided
	var birthDate *time.Time
	if req.BirthDate != nil && *req.BirthDate != "" {
		parsed, err := time.Parse("2006-01-02", *req.BirthDate)
		if err != nil {
			return nil, errors.New("invalid birth_date format, use YYYY-MM-DD")
		}
		birthDate = &parsed
	}

	// 5. Create user
	user := &model.User{
		Email:       req.Email,
		FullName:    req.FullName,
		PhoneNumber: req.PhoneNumber,
		BirthDate:   birthDate,
		RoleID:      &req.RoleID,
		IsActive:    true,
	}
	user.CreatedBy = creatorID
	user.UpdatedBy = creatorID

	// 6. Set password
	if err := user.SetPassword(req.Password); err != nil {
		return nil, errors.New("failed to hash password")
	}

	// 7. Auto-assign privileges based on role
	user.Privileges = role.Privileges

	// 8. Save to database
	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *userService) UpdateUser(userID uuid.UUID, req *UpdateUserRequest, updaterID string) (*model.User, error) {
	// 1. Validate request
	if errs := validator.ValidateStruct(req); len(errs) > 0 {
		firstErr := errs[0]
		return nil, fmt.Errorf("Validation failed: Field '%s' failed on tag '%s'", firstErr.FailedField, firstErr.Tag)
	}

	// 2. Find existing user
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 3. Check if email is being changed and already exists
	if req.Email != user.Email {
		existing, _ := s.userRepo.FindByEmail(req.Email)
		if existing != nil {
			return nil, ErrEmailExists
		}
	}

	// 4. Validate role exists
	role, err := s.roleRepo.FindByID(req.RoleID)
	if err != nil {
		return nil, errors.New("role not found")
	}

	// 5. Parse birthdate if provided
	var birthDate *time.Time
	if req.BirthDate != nil && *req.BirthDate != "" {
		parsed, err := time.Parse("2006-01-02", *req.BirthDate)
		if err != nil {
			return nil, errors.New("invalid birth_date format, use YYYY-MM-DD")
		}
		birthDate = &parsed
	}

	// 6. Update user fields
	user.Email = req.Email
	user.FullName = req.FullName
	user.PhoneNumber = req.PhoneNumber
	user.BirthDate = birthDate
	user.RoleID = &req.RoleID
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	user.UpdatedBy = updaterID

	// 7. Update password if provided
	if req.Password != nil && *req.Password != "" {
		if err := user.SetPassword(*req.Password); err != nil {
			return nil, errors.New("failed to hash password")
		}
	}

	// 8. Auto-update privileges based on role
	user.Privileges = role.Privileges

	// 9. Save to database
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	// 10. Reload and return
	return s.userRepo.FindByID(userID)
}

func (s *userService) DeleteUser(userID uuid.UUID) error {
	return s.userRepo.Delete(userID)
}

func (s *userService) UpdateUserPrivileges(userID uuid.UUID, privilegeCodes []string, updaterID string) (*model.User, error) {
	// 1. Find user
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 2. Get privileges
	privileges, err := s.privilegeRepo.FindByCodes(privilegeCodes)
	if err != nil {
		return nil, errors.New("failed to find privileges")
	}

	// 3. Update privileges
	if err := s.userRepo.UpdatePrivileges(userID, privileges); err != nil {
		return nil, err
	}

	// 4. Update audit field
	user.UpdatedBy = updaterID
	s.userRepo.Update(user)

	// 5. Reload user with updated privileges
	return s.userRepo.FindByID(userID)
}

func (s *userService) GetAllUsers() ([]model.UserResponse, error) {
	users, err := s.userRepo.FindAll()
	if err != nil {
		return nil, err
	}

	responses := make([]model.UserResponse, len(users))
	for i, user := range users {
		responses[i] = user.ToResponse()
	}
	return responses, nil
}

func (s *userService) GetUserByID(id uuid.UUID) (*model.UserResponse, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, ErrUserNotFound
	}
	response := user.ToResponse()
	return &response, nil
}
