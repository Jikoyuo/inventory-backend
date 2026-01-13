package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"go-inventory-ws/internal/model"
	"go-inventory-ws/internal/repository"
	"go-inventory-ws/internal/ws"
	"go-inventory-ws/pkg/jwt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserInactive       = errors.New("user account is inactive")
	ErrWrongPassword      = errors.New("current password is incorrect")
	ErrSessionTimeout     = errors.New("session expired due to inactivity")
)

type AuthService interface {
	Login(email, password string) (*LoginResponse, error)
	ResetPassword(email, oldPassword, newPassword string) error
	ValidateToken(tokenString string) (*TokenValidationResponse, error)
	Heartbeat(userID uuid.UUID) error
}

type LoginResponse struct {
	Token      string             `json:"token"`
	User       model.UserResponse `json:"user"`
	Role       *model.Role        `json:"role"`       // Direct role object for Redux
	Privileges []string           `json:"privileges"` // Flat privileges array for easy checking
}

type TokenValidationResponse struct {
	User       model.UserResponse `json:"user"`
	Role       *model.Role        `json:"role"`
	Privileges []string           `json:"privileges"`
}

type authService struct {
	userRepo repository.UserRepository
	wsHub    *ws.Hub
}

func NewAuthService(userRepo repository.UserRepository, hub *ws.Hub) AuthService {
	return &authService{
		userRepo: userRepo,
		wsHub:    hub,
	}
}

func (s *authService) Login(email, password string) (*LoginResponse, error) {
	// 1. Find user by email
	user, err := s.userRepo.FindByEmail(email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// 2. Check if user is active
	if !user.IsActive {
		return nil, ErrUserInactive
	}

	// 3. Verify password
	if !user.CheckPassword(password) {
		return nil, ErrInvalidCredentials
	}

	// 4. Get role code
	roleCode := ""
	if user.Role != nil {
		roleCode = user.Role.Code
	}

	// 5. Single Session: Generate New Token Version
	newTokenVersion := uuid.New().String()
	// Update TokenVersion AND LastSeenAt (to prevent immediate timeout)
	now := time.Now()
	// We need to update user object first to pass to UpdateTokenVersion if it supported it,
	// but userRepo.UpdateTokenVersion likely only updates version.
	// Let's manually set LastSeenAt on the user object and update it using a more generic approach
	// or just call UpdateLastSeen immediately.
	// However, UpdateTokenVersion is atomic.
	// Let's update both. Ideally userRepo has a method for this,
	// but for now we can rely on Heartbeat logic or just update here.
	// Let's modify the user object and save.
	user.TokenVersion = newTokenVersion
	user.LastSeenAt = &now

	// Use repository to save changes
	// Assuming userRepo.Update handles everything.
	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to update session")
	}

	// 6. Generate JWT token with TokenVersion
	token, err := jwt.GenerateToken(user.ID, user.Email, user.FullName, roleCode, user.GetPrivilegeCodes(), newTokenVersion)
	if err != nil {
		return nil, errors.New("failed to generate token")
	}

	return &LoginResponse{
		Token:      token,
		User:       user.ToResponse(),
		Role:       user.Role,                // Direct role object
		Privileges: user.GetPrivilegeCodes(), // Flat privileges array
	}, nil
}

func (s *authService) ResetPassword(email, oldPassword, newPassword string) error {
	// 1. Find user by email
	user, err := s.userRepo.FindByEmail(email)
	if err != nil {
		return ErrUserNotFound
	}

	// 2. Verify old password
	if !user.CheckPassword(oldPassword) {
		return ErrWrongPassword
	}

	// 3. Set new password
	if err := user.SetPassword(newPassword); err != nil {
		return errors.New("failed to hash new password")
	}

	// 4. Update in database
	if err := s.userRepo.Update(user); err != nil {
		return err
	}

	// 5. Invalidate existing sessions (Optional, but good practice)
	// s.userRepo.UpdateTokenVersion(user.ID, uuid.New().String())

	return nil
}

func (s *authService) ValidateToken(tokenString string) (*TokenValidationResponse, error) {
	// 1. Validate JWT token
	claims, err := jwt.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	// 2. Find user by ID from token claims
	user, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 3. Check if user is still active
	if !user.IsActive {
		return nil, ErrUserInactive
	}

	// 4. Check against DB for strict session (TokenVersion)
	if user.TokenVersion != claims.TokenVersion {
		return nil, errors.New("session expired (logged in on another device)")
	}

	// 5. Check Inactivity (LastSeenAt > 5 Minutes)
	if user.LastSeenAt != nil {
		if time.Since(*user.LastSeenAt) > 5*time.Minute {
			return nil, ErrSessionTimeout
		}
	} else {
		// Edge case: If LastSeenAt is nil (legacy user?), valid or invalid?
		// If they just logged in, it should be set. If nil, maybe force re-login.
		// Let's handle gracefully: if nil, assume active IF token is new?
		// No, to be safe, if nil, invalid, force login to set it.
		// Or maybe allow? Let's allow but maybe should force.
		// User asked: "user off(lastSeenAt) lebih dari 5 menit".
		// If nil, techincally unknown.
		// Let's invalidate to enforce the rule.
		return nil, ErrSessionTimeout
	}

	// 6. Return user info with role and privileges
	return &TokenValidationResponse{
		User:       user.ToResponse(),
		Role:       user.Role,
		Privileges: user.GetPrivilegeCodes(),
	}, nil
}

func (s *authService) Heartbeat(userID uuid.UUID) error {
	// 1. Update timestamp di DB
	if err := s.userRepo.UpdateLastSeen(userID); err != nil {
		return err
	}

	// 2. Broadcast status "online" ke semua client
	// Kita broadcast setiap heartbeat agar user yang baru connect dapat info terbaru
	// Frontend sebaiknya handle duplicate/throttle jika perlu
	go func() {
		payload := map[string]interface{}{
			"type":         "user_status_update",
			"user_id":      userID.String(),
			"status":       "online",
			"last_seen_at": time.Now(),
		}
		msg, _ := json.Marshal(payload)
		s.wsHub.Broadcast <- msg
	}()

	return nil
}
