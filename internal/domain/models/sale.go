package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	SaleStatusPending   = "pending"
	SaleStatusCompleted = "completed"
	SaleStatusPartial   = "partial"
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
	// ── Primary key ───────────────────────────────────────────────────────────
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`

	// ── Foreign keys ──────────────────────────────────────────────────────────
	// BusinessID is part of both composite unique indexes below, so it carries
	// two uniqueIndex names in addition to its own plain index.
	BusinessID uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_sales_business_sale_number;uniqueIndex:idx_sales_business_receipt_number" json:"business_id"`
	StoreID    *uuid.UUID `gorm:"type:uuid;index"                                                                                                     json:"store_id"`
	CustomerID *uuid.UUID `gorm:"type:uuid;index"                                                                                                     json:"customer_id"`
	InvoiceID  *uuid.UUID `gorm:"type:uuid;index"                                                                                                     json:"invoice_id"`
	RecordedBy uuid.UUID  `gorm:"type:uuid;not null"                                                                                                  json:"recorded_by"`
	StaffName  *string    `gorm:"size:255"                                                                                                            json:"staff_name,omitempty"`

	// ── Sale / receipt numbers ────────────────────────────────────────────────
	// Both are unique PER BUSINESS (composite index with business_id), NOT
	// globally unique. This allows every business to have their own SL-000001,
	// SL-000002, … sequence without conflicting with other businesses.
	SaleNumber    string  `gorm:"size:50;not null;uniqueIndex:idx_sales_business_sale_number"  json:"sale_number"`
	ReceiptNumber *string `gorm:"size:50;uniqueIndex:idx_sales_business_receipt_number"         json:"receipt_number"`

	// ── Idempotency ───────────────────────────────────────────────────────────
	IdempotencyKey *uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"-"`

	// ── Financials ────────────────────────────────────────────────────────────
	SubTotal       int64   `gorm:"not null"        json:"sub_total"`
	DiscountAmount int64   `gorm:"default:0"       json:"discount_amount"`
	VATRate        float64 `gorm:"default:0"       json:"vat_rate"`
	VATInclusive   bool    `gorm:"default:false"   json:"vat_inclusive"`
	VATAmount      int64   `gorm:"default:0"       json:"vat_amount"`
	WHTRate        float64 `gorm:"default:0"       json:"wht_rate"`
	WHTAmount      int64   `gorm:"default:0"       json:"wht_amount"`
	TotalAmount    int64   `gorm:"not null"        json:"total_amount"`
	AmountPaid     int64   `gorm:"default:0"       json:"amount_paid"`
	BalanceDue     int64   `gorm:"default:0"       json:"balance_due"`

	// ── Payment / status ──────────────────────────────────────────────────────
	PaymentMethod string     `gorm:"size:30;not null"              json:"payment_method"`
	Status        string     `gorm:"size:20;not null;default:'completed'" json:"status"`
	Notes         *string    `gorm:"type:text"                     json:"notes,omitempty"`
	ReceiptSentAt *time.Time `                                      json:"receipt_sent_at"`

	// ── Timestamps ────────────────────────────────────────────────────────────
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// ── Associations ──────────────────────────────────────────────────────────
	Business  Business   `gorm:"foreignKey:BusinessID" json:"-"`
	Store     *Store     `gorm:"foreignKey:StoreID"    json:"store,omitempty"`
	Customer  *Customer  `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Invoice   *Invoice   `gorm:"foreignKey:InvoiceID"  json:"invoice,omitempty"`
	SaleItems []SaleItem `gorm:"foreignKey:SaleID"     json:"items,omitempty"`
}

// ─── Tax / total helpers ──────────────────────────────────────────────────────

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
	if net < 0 {
		net = 0
	}
	s.TotalAmount = net
	s.BalanceDue = s.TotalAmount - s.AmountPaid
	if s.BalanceDue < 0 {
		s.BalanceDue = 0
	}
}
