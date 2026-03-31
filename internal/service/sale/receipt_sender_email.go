package sale

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/email"
)

type EmailReceiptSender struct {
	db *gorm.DB
}

func NewEmailReceiptSender(db *gorm.DB) *EmailReceiptSender {
	return &EmailReceiptSender{db: db}
}

func (s *EmailReceiptSender) SendSaleReceipt(payload ReceiptEmailPayload) error {
	if strings.TrimSpace(payload.ToEmail) == "" {
		return nil
	}

	var business models.Business
	if err := s.db.
		Select("id, name").
		Where("id = ?", payload.BusinessID).
		First(&business).Error; err != nil {
		return fmt.Errorf("load business for receipt email: %w", err)
	}

	sale := &models.Sale{
		ID:            payload.SaleID,
		BusinessID:    payload.BusinessID,
		SaleNumber:    payload.SaleNumber,
		ReceiptNumber: &payload.ReceiptNumber,
		PaymentMethod: payload.PaymentMethod,
		AmountPaid:    payload.AmountPaid,
		BalanceDue:    payload.BalanceDue,
		TotalAmount:   payload.TotalAmount,
		CreatedAt:     payload.CreatedAt,
		Notes:         payload.Notes,
		SaleItems:     payload.Items,
		Customer: &models.Customer{
			FirstName: payload.CustomerFirstName,
			LastName:  payload.CustomerLastName,
			Email:     &payload.ToEmail,
		},
	}

	return email.SendReceiptEmail(payload.ToEmail, sale, business.Name)
}

var _ ReceiptSender = (*EmailReceiptSender)(nil)
