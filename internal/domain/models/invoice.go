// internal/domain/models/invoice.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	InvoiceStatusDraft     = "draft"
	InvoiceStatusSent      = "sent"
	InvoiceStatusPaid      = "paid"
	InvoiceStatusPartial   = "partial"
	InvoiceStatusOverdue   = "overdue"
	InvoiceStatusCancelled = "cancelled"
)

type Invoice struct {
	ID                uuid.UUID     `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	BusinessID        uuid.UUID     `gorm:"type:uuid;not null;index" json:"business_id"`
	StoreID           *uuid.UUID    `gorm:"type:uuid;index" json:"store_id"`
	CustomerID        *uuid.UUID    `gorm:"type:uuid;index" json:"customer_id"`
	CreatedBy         uuid.UUID     `gorm:"type:uuid;not null" json:"created_by"`
	InvoiceNumber     string        `gorm:"size:50;uniqueIndex;not null" json:"invoice_number"`
	IssueDate         time.Time     `gorm:"not null" json:"issue_date"`
	DueDate           time.Time     `gorm:"not null;index" json:"due_date"`
	SubTotal          int64         `gorm:"not null" json:"sub_total"`
	DiscountAmount    int64         `gorm:"default:0" json:"discount_amount"`
	VATRate           float64       `gorm:"default:0" json:"vat_rate"`
	VATInclusive      bool          `gorm:"default:false" json:"vat_inclusive"`
	VATAmount         int64         `gorm:"default:0" json:"vat_amount"`
	WHTRate           float64       `gorm:"default:0" json:"wht_rate"`
	WHTAmount         int64         `gorm:"default:0" json:"wht_amount"`
	TotalAmount       int64         `gorm:"not null" json:"total_amount"`
	AmountPaid        int64         `gorm:"default:0" json:"amount_paid"`
	BalanceDue        int64         `gorm:"default:0" json:"balance_due"`
	Currency          string        `gorm:"size:5;default:'NGN'" json:"currency"`
	Status            string        `gorm:"size:20;not null;default:'draft'" json:"status"`
	Notes             *string       `gorm:"type:text" json:"notes"`
	PaymentTerms      *string       `gorm:"size:500" json:"payment_terms"`
	SentAt            *time.Time    `json:"sent_at"`
	PaidAt            *time.Time    `json:"paid_at"`
	ConvertedToSaleID *uuid.UUID    `gorm:"type:uuid" json:"converted_to_sale_id"`
	CreatedAt         time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time     `gorm:"autoUpdateTime" json:"updated_at"`
	Business          Business      `gorm:"foreignKey:BusinessID" json:"-"`
	Customer          *Customer     `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	InvoiceItems      []InvoiceItem `gorm:"foreignKey:InvoiceID" json:"items,omitempty"`
}

func (inv *Invoice) CalculateVAT() {
	if inv.VATRate == 0 {
		inv.VATAmount = 0
		return
	}
	net := inv.SubTotal - inv.DiscountAmount
	if inv.VATInclusive {
		base := float64(net) / (1 + inv.VATRate/100)
		inv.VATAmount = int64(float64(net) - base)
	} else {
		inv.VATAmount = int64(float64(net) * inv.VATRate / 100)
	}
}

func (inv *Invoice) CalculateWHT() {
	if inv.WHTRate == 0 {
		inv.WHTAmount = 0
		return
	}
	net := inv.SubTotal - inv.DiscountAmount
	inv.WHTAmount = int64(float64(net) * inv.WHTRate / 100)
}

func (inv *Invoice) CalculateTotal() {
	inv.CalculateVAT()
	inv.CalculateWHT()
	net := inv.SubTotal - inv.DiscountAmount
	if !inv.VATInclusive {
		net += inv.VATAmount
	}
	net -= inv.WHTAmount
	inv.TotalAmount = net
	inv.BalanceDue = inv.TotalAmount - inv.AmountPaid
}

func (inv *Invoice) UpdateStatus() {
	if inv.Status == InvoiceStatusCancelled {
		return
	}
	switch {
	case inv.AmountPaid >= inv.TotalAmount && inv.TotalAmount > 0:
		inv.Status = InvoiceStatusPaid
		inv.BalanceDue = 0
	case inv.AmountPaid > 0:
		inv.Status = InvoiceStatusPartial
		inv.BalanceDue = inv.TotalAmount - inv.AmountPaid
	case inv.Status == InvoiceStatusSent && time.Now().After(inv.DueDate):
		inv.Status = InvoiceStatusOverdue
	}
}
