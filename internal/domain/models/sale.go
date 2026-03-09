// internal/domain/models/sale.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	SaleStatusPending   = "pending"
	SaleStatusCompleted = "completed"
	SaleStatusCancelled = "cancelled"

	PaymentMethodCash        = "cash"
	PaymentMethodTransfer    = "transfer"
	PaymentMethodPOS         = "pos"
	PaymentMethodPaystack    = "paystack"
	PaymentMethodFlutterwave = "flutterwave"
	PaymentMethodCredit      = "credit"

	StandardVATRate = 7.5
)

type Sale struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	StoreID    *uuid.UUID `gorm:"type:uuid;index" json:"store_id"`
	CustomerID *uuid.UUID `gorm:"type:uuid;index" json:"customer_id"`
	InvoiceID  *uuid.UUID `gorm:"type:uuid;index" json:"invoice_id"`
	RecordedBy uuid.UUID  `gorm:"type:uuid;not null" json:"recorded_by"`

	SaleNumber    string  `gorm:"size:50;uniqueIndex;not null" json:"sale_number"`
	ReceiptNumber *string `gorm:"size:50;uniqueIndex" json:"receipt_number"`

	SubTotal       int64   `gorm:"not null" json:"sub_total"`
	DiscountAmount int64   `gorm:"default:0" json:"discount_amount"`
	VATRate        float64 `gorm:"default:0" json:"vat_rate"`
	VATInclusive   bool    `gorm:"default:false" json:"vat_inclusive"`
	VATAmount      int64   `gorm:"default:0" json:"vat_amount"`
	WHTRate        float64 `gorm:"default:0" json:"wht_rate"`
	WHTAmount      int64   `gorm:"default:0" json:"wht_amount"`
	TotalAmount    int64   `gorm:"not null" json:"total_amount"`
	AmountPaid     int64   `gorm:"default:0" json:"amount_paid"`
	BalanceDue     int64   `gorm:"default:0" json:"balance_due"`

	PaymentMethod string  `gorm:"size:30;not null" json:"payment_method"`
	Status        string  `gorm:"size:20;not null;default:'completed'" json:"status"`
	Notes         *string `gorm:"type:text" json:"notes"`

	ReceiptSentAt *time.Time `json:"receipt_sent_at"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	Business  Business   `gorm:"foreignKey:BusinessID" json:"-"`
	Store     *Store     `gorm:"foreignKey:StoreID" json:"store,omitempty"`
	Customer  *Customer  `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Invoice   *Invoice   `gorm:"foreignKey:InvoiceID" json:"invoice,omitempty"`
	SaleItems []SaleItem `gorm:"foreignKey:SaleID" json:"items,omitempty"`
}

func (s *Sale) CalculateVAT() {
	if s.VATRate == 0 {
		s.VATAmount = 0
		return
	}
	if s.VATInclusive {
		base := float64(s.SubTotal-s.DiscountAmount) / (1 + s.VATRate/100)
		s.VATAmount = int64(float64(s.SubTotal-s.DiscountAmount) - base)
	} else {
		s.VATAmount = int64(float64(s.SubTotal-s.DiscountAmount) * s.VATRate / 100)
	}
}

func (s *Sale) CalculateWHT() {
	if s.WHTRate == 0 {
		s.WHTAmount = 0
		return
	}
	s.WHTAmount = int64(float64(s.SubTotal-s.DiscountAmount) * s.WHTRate / 100)
}

func (s *Sale) CalculateTotal() {
	s.CalculateVAT()
	s.CalculateWHT()
	net := s.SubTotal - s.DiscountAmount
	if !s.VATInclusive {
		net += s.VATAmount
	}
	net -= s.WHTAmount
	s.TotalAmount = net
	s.BalanceDue = s.TotalAmount - s.AmountPaid
}
