package model

type Product struct {
	BaseModel
	SKU   string `gorm:"type:varchar(50);uniqueIndex;not null" json:"sku" validate:"required"`
	Name  string `gorm:"type:varchar(255);not null" json:"name" validate:"required"`
	Stock int    `gorm:"default:0" json:"stock"`
	Unit  string `gorm:"type:varchar(20)" json:"unit"`
	Price int64  `gorm:"default:0" json:"price"`

	// User tracking
	CreatedByUserID *string `gorm:"type:varchar(255)" json:"created_by_user_id,omitempty"`
	UpdatedByUserID *string `gorm:"type:varchar(255)" json:"updated_by_user_id,omitempty"`
	CreatedByUser   *User   `gorm:"foreignKey:CreatedByUserID;references:ID" json:"created_by_user,omitempty"`
	UpdatedByUser   *User   `gorm:"foreignKey:UpdatedByUserID;references:ID" json:"updated_by_user,omitempty"`

	// Relasi
	Transactions []Transaction `json:"transactions,omitempty"`
}
