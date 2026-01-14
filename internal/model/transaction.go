package model

import "github.com/google/uuid"

type TransactionType string

const (
	TxIn  TransactionType = "IN"
	TxOut TransactionType = "OUT"
)

type Transaction struct {
	BaseModel
	ProductID     uuid.UUID       `gorm:"type:uuid;not null" json:"product_id" validate:"uuid_required"`
	Product       Product         `json:"product" validate:"-"` // Relasi - skip validation
	Type          TransactionType `gorm:"type:varchar(10);not null" json:"type" validate:"required,oneof=IN OUT"`
	Quantity      int             `gorm:"not null" json:"quantity" validate:"required,gt=0"` // Qty harus > 0
	TotalAmount   int64           `gorm:"not null" json:"total_amount"`                      // Snapshot price * quantity
	PaymentMethod string          `gorm:"type:varchar(20)" json:"payment_method"`            // CASH, TRANSFER. Bisa kosong/0 logic.
	Note          string          `json:"note"`

	// User tracking
	CreatedByUserID *string `gorm:"type:varchar(255)" json:"created_by_user_id,omitempty"`
	CreatedByUser   *User   `gorm:"foreignKey:CreatedByUserID;references:ID" json:"created_by_user,omitempty"`
}
