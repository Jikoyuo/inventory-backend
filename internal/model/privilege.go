package model

// Privilege represents a permission that can be assigned to users
type Privilege struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Code string `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"` // e.g., "product:create"
	Name string `gorm:"type:varchar(100)" json:"name"`                     // e.g., "Create Product"
}

// Default privileges for the system
var DefaultPrivileges = []Privilege{
	// User management
	{Code: "user:view", Name: "View User"},
	{Code: "user:create", Name: "Create User"},
	{Code: "user:update", Name: "Update User"},
	{Code: "user:delete", Name: "Delete User"},
	{Code: "user:update_privilege", Name: "Update User Privileges"},
	// Product management
	{Code: "product:view", Name: "View Product"},
	{Code: "product:create", Name: "Create Product"},
	{Code: "product:update", Name: "Update Product"},
	{Code: "product:delete", Name: "Delete Product"},
	// Transaction management
	{Code: "transaction:view", Name: "View Transaction"},
	{Code: "transaction:create", Name: "Create Transaction"},
	// Dashboard
	{Code: "dashboard:view", Name: "View Dashboard"},
	// Shift management (MASTER_ADMIN only)
	{Code: "shift:view", Name: "View Shift"},
	{Code: "shift:create", Name: "Create Shift"},
	{Code: "shift:update", Name: "Update Shift"},
	{Code: "shift:delete", Name: "Delete Shift"},
}
