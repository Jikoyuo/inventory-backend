package model

// Role represents user roles in the system
type Role struct {
	ID          uint        `gorm:"primaryKey" json:"id"`
	Code        string      `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"` // MASTER_ADMIN, ADMIN
	Name        string      `gorm:"type:varchar(100)" json:"name"`
	Description string      `gorm:"type:text" json:"description"`
	Privileges  []Privilege `gorm:"many2many:role_privileges;" json:"privileges,omitempty"`
}

// Role codes as constants
const (
	RoleMasterAdmin = "MASTER_ADMIN"
	RoleAdmin       = "ADMIN"
)

// DefaultRoles defines the default roles in the system
var DefaultRoles = []Role{
	{
		Code:        RoleMasterAdmin,
		Name:        "Master Administrator",
		Description: "Full system access with all privileges",
	},
	{
		Code:        RoleAdmin,
		Name:        "Administrator",
		Description: "Limited administrative access",
	},
}
