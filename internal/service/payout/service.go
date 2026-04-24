package payout

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/external/paystack"
	"ogaos-backend/internal/pkg/email"
	"ogaos-backend/internal/pkg/otp"
)

const (
	otpExpiryMinutes  = 5
	otpMaxAttempts    = 5
	otpResendCooldown = 60 * time.Second
	defaultCurrency   = "NGN"
	ownerRole         = "owner"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

type OwnerContact struct {
	UserID    uuid.UUID
	Email     string
	FirstName string
}

type ResolveAccountRequest struct {
	BankCode      string `json:"bank_code"`
	AccountNumber string `json:"account_number"`
}

type StartVerificationRequest struct {
	BankName      string `json:"bank_name"`
	BankCode      string `json:"bank_code"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
}

type ConfirmVerificationRequest struct {
	VerificationID uuid.UUID `json:"verification_id"`
	OTP            string    `json:"otp"`
}

type VerificationResponse struct {
	ID            uuid.UUID `json:"id"`
	BusinessID    uuid.UUID `json:"business_id"`
	BankName      string    `json:"bank_name"`
	BankCode      string    `json:"bank_code"`
	AccountNumber string    `json:"account_number"`
	AccountName   string    `json:"account_name"`
	ExpiresAt     time.Time `json:"expires_at"`
	ResendAfter   time.Time `json:"resend_after"`
}

type PayoutAccountResponse struct {
	ID            uuid.UUID `json:"id"`
	BusinessID    uuid.UUID `json:"business_id"`
	BankName      string    `json:"bank_name"`
	BankCode      string    `json:"bank_code"`
	AccountNumber string    `json:"account_number"`
	AccountName   string    `json:"account_name"`
	IsVerified    bool      `json:"is_verified"`
	IsDefault     bool      `json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (s *Service) ListBanks() (*paystack.ListBanksResponse, error) {
	client, err := s.paystackClient()
	if err != nil {
		return nil, err
	}
	return client.ListBanks()
}

func (s *Service) ResolveAccount(req ResolveAccountRequest) (*paystack.ResolveAccountResponse, error) {
	req.BankCode = strings.TrimSpace(req.BankCode)
	req.AccountNumber = strings.TrimSpace(req.AccountNumber)

	if req.BankCode == "" {
		return nil, errors.New("bank_code is required")
	}
	if req.AccountNumber == "" {
		return nil, errors.New("account_number is required")
	}
	if len(req.AccountNumber) != 10 {
		return nil, errors.New("account_number must be 10 digits")
	}

	client, err := s.paystackClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.ResolveAccount(req.AccountNumber, req.BankCode)
	if err != nil {
		return nil, err
	}
	if !resp.Status {
		return nil, errors.New(resp.Message)
	}

	return resp, nil
}

func (s *Service) StartVerification(businessID uuid.UUID, req StartVerificationRequest) (*VerificationResponse, error) {
	req.BankName = strings.TrimSpace(req.BankName)
	req.BankCode = strings.TrimSpace(req.BankCode)
	req.AccountNumber = strings.TrimSpace(req.AccountNumber)
	req.AccountName = strings.TrimSpace(req.AccountName)

	if req.BankName == "" {
		return nil, errors.New("bank_name is required")
	}
	if req.BankCode == "" {
		return nil, errors.New("bank_code is required")
	}
	if req.AccountNumber == "" {
		return nil, errors.New("account_number is required")
	}
	if len(req.AccountNumber) != 10 {
		return nil, errors.New("account_number must be 10 digits")
	}

	owner, err := s.getOwnerContact(businessID)
	if err != nil {
		return nil, err
	}

	// Always re-resolve on the server. Do not trust the frontend-sent account name.
	resolveResp, err := s.ResolveAccount(ResolveAccountRequest{
		BankCode:      req.BankCode,
		AccountNumber: req.AccountNumber,
	})
	if err != nil {
		return nil, err
	}

	accountName := strings.TrimSpace(resolveResp.Data.AccountName)
	if accountName == "" {
		return nil, errors.New("failed to verify account name")
	}

	now := time.Now()

	// Allow only one active pending verification per business.
	var existing models.PayoutAccountVerification
	err = s.db.
		Where("business_id = ? AND is_verified = ?", businessID, false).
		Order("created_at DESC").
		First(&existing).Error
	if err == nil {
		if now.Before(existing.ResendAfter) {
			return nil, fmt.Errorf("please wait until %s before requesting another otp", existing.ResendAfter.Format(time.RFC3339))
		}
		if err := s.db.Delete(&existing).Error; err != nil {
			return nil, err
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	code, err := otp.GenerateOTP()
	if err != nil {
		return nil, err
	}

	verification := models.PayoutAccountVerification{
		BusinessID:    businessID,
		BankName:      req.BankName,
		BankCode:      req.BankCode,
		AccountNumber: req.AccountNumber,
		AccountName:   accountName,
		OTPHash:       otp.HashOTP(code),
		ExpiresAt:     now.Add(otpExpiryMinutes * time.Minute),
		ResendAfter:   now.Add(otpResendCooldown),
		Attempts:      0,
		MaxAttempts:   otpMaxAttempts,
		IsVerified:    false,
	}

	if err := s.db.Create(&verification).Error; err != nil {
		return nil, err
	}

	email.SendBusinessPayoutOTP(
		owner.Email,
		owner.FirstName,
		code,
		verification.BankName,
		verification.AccountNumber,
	)

	return &VerificationResponse{
		ID:            verification.ID,
		BusinessID:    verification.BusinessID,
		BankName:      verification.BankName,
		BankCode:      verification.BankCode,
		AccountNumber: verification.AccountNumber,
		AccountName:   verification.AccountName,
		ExpiresAt:     verification.ExpiresAt,
		ResendAfter:   verification.ResendAfter,
	}, nil
}

func (s *Service) ResendVerification(businessID uuid.UUID, verificationID uuid.UUID) (*VerificationResponse, error) {
	var verification models.PayoutAccountVerification
	if err := s.db.
		Where("id = ? AND business_id = ? AND is_verified = ?", verificationID, businessID, false).
		First(&verification).Error; err != nil {
		return nil, errors.New("verification session not found")
	}

	now := time.Now()

	if now.After(verification.ExpiresAt) {
		_ = s.db.Delete(&verification).Error
		return nil, errors.New("verification session has expired")
	}

	if now.Before(verification.ResendAfter) {
		return nil, fmt.Errorf("please wait until %s before requesting another otp", verification.ResendAfter.Format(time.RFC3339))
	}

	owner, err := s.getOwnerContact(businessID)
	if err != nil {
		return nil, err
	}

	code, err := otp.GenerateOTP()
	if err != nil {
		return nil, err
	}

	verification.OTPHash = otp.HashOTP(code)
	verification.ResendAfter = now.Add(otpResendCooldown)
	verification.ExpiresAt = now.Add(otpExpiryMinutes * time.Minute)
	verification.Attempts = 0

	if err := s.db.Save(&verification).Error; err != nil {
		return nil, err
	}

	email.SendBusinessPayoutOTP(
		owner.Email,
		owner.FirstName,
		code,
		verification.BankName,
		verification.AccountNumber,
	)

	return &VerificationResponse{
		ID:            verification.ID,
		BusinessID:    verification.BusinessID,
		BankName:      verification.BankName,
		BankCode:      verification.BankCode,
		AccountNumber: verification.AccountNumber,
		AccountName:   verification.AccountName,
		ExpiresAt:     verification.ExpiresAt,
		ResendAfter:   verification.ResendAfter,
	}, nil
}

func (s *Service) ConfirmVerification(businessID uuid.UUID, req ConfirmVerificationRequest) (*PayoutAccountResponse, error) {
	if req.VerificationID == uuid.Nil {
		return nil, errors.New("verification_id is required")
	}

	req.OTP = strings.TrimSpace(req.OTP)
	if req.OTP == "" {
		return nil, errors.New("otp is required")
	}

	var verification models.PayoutAccountVerification
	if err := s.db.
		Where("id = ? AND business_id = ? AND is_verified = ?", req.VerificationID, businessID, false).
		First(&verification).Error; err != nil {
		return nil, errors.New("verification session not found")
	}

	now := time.Now()

	if now.After(verification.ExpiresAt) {
		_ = s.db.Delete(&verification).Error
		return nil, errors.New("verification session has expired")
	}

	if verification.Attempts >= verification.MaxAttempts {
		_ = s.db.Delete(&verification).Error
		return nil, errors.New("maximum otp attempts exceeded")
	}

	if !otp.VerifyOTP(req.OTP, verification.OTPHash) {
		verification.Attempts++
		if err := s.db.Model(&verification).Update("attempts", verification.Attempts).Error; err != nil {
			return nil, err
		}

		if verification.Attempts >= verification.MaxAttempts {
			_ = s.db.Delete(&verification).Error
			return nil, errors.New("maximum otp attempts exceeded")
		}

		return nil, errors.New("invalid otp")
	}

	client, err := s.paystackClient()
	if err != nil {
		return nil, err
	}

	recipientResp, err := client.CreateRecipient(paystack.CreateRecipientRequest{
		Type:          "nuban",
		Name:          verification.AccountName,
		AccountNumber: verification.AccountNumber,
		BankCode:      verification.BankCode,
		Currency:      defaultCurrency,
	})
	if err != nil {
		return nil, err
	}
	if !recipientResp.Status {
		return nil, errors.New(recipientResp.Message)
	}

	var saved models.BusinessPayoutAccount
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Deactivate any prior default payout accounts for this business.
		if err := tx.Model(&models.BusinessPayoutAccount{}).
			Where("business_id = ?", businessID).
			Updates(map[string]interface{}{
				"is_default": false,
			}).Error; err != nil {
			return err
		}

		saved = models.BusinessPayoutAccount{
			BusinessID:    businessID,
			BankName:      verification.BankName,
			BankCode:      verification.BankCode,
			AccountNumber: verification.AccountNumber,
			AccountName:   verification.AccountName,
			PaystackRecipientCode: func(v string) *string {
				return &v
			}(recipientResp.Data.RecipientCode),
			IsVerified: true,
			IsDefault:  true,
		}

		if err := tx.Create(&saved).Error; err != nil {
			return err
		}

		if err := tx.Model(&verification).Update("is_verified", true).Error; err != nil {
			return err
		}

		if err := tx.Delete(&verification).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.toPayoutAccountResponse(&saved), nil
}

func (s *Service) GetDefaultAccount(businessID uuid.UUID) (*PayoutAccountResponse, error) {
	var acct models.BusinessPayoutAccount
	if err := s.db.
		Where("business_id = ? AND is_default = ?", businessID, true).
		First(&acct).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("payout account not found")
		}
		return nil, err
	}

	return s.toPayoutAccountResponse(&acct), nil
}

func (s *Service) GetPendingVerification(businessID uuid.UUID) (*VerificationResponse, error) {
	var verification models.PayoutAccountVerification
	if err := s.db.
		Where("business_id = ? AND is_verified = ?", businessID, false).
		Order("created_at DESC").
		First(&verification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no pending verification found")
		}
		return nil, err
	}

	if time.Now().After(verification.ExpiresAt) {
		_ = s.db.Delete(&verification).Error
		return nil, errors.New("verification session has expired")
	}

	return &VerificationResponse{
		ID:            verification.ID,
		BusinessID:    verification.BusinessID,
		BankName:      verification.BankName,
		BankCode:      verification.BankCode,
		AccountNumber: verification.AccountNumber,
		AccountName:   verification.AccountName,
		ExpiresAt:     verification.ExpiresAt,
		ResendAfter:   verification.ResendAfter,
	}, nil
}

func (s *Service) paystackClient() (*paystack.Client, error) {
	secret := strings.TrimSpace(os.Getenv("PAYSTACK_SECRET_KEY"))
	if secret == "" {
		return nil, errors.New("PAYSTACK_SECRET_KEY is not set")
	}
	return paystack.NewClient(secret), nil
}

func (s *Service) getOwnerContact(businessID uuid.UUID) (*OwnerContact, error) {
	var owner OwnerContact

	err := s.db.
		Table("business_users").
		Select(`
			users.id as user_id,
			users.email as email,
			COALESCE(users.first_name, 'there') as first_name
		`).
		Joins("JOIN users ON users.id = business_users.user_id").
		Where("business_users.business_id = ? AND business_users.role = ? AND business_users.is_active = ?", businessID, ownerRole, true).
		Limit(1).
		Scan(&owner).Error
	if err != nil {
		return nil, err
	}

	if owner.UserID == uuid.Nil || strings.TrimSpace(owner.Email) == "" {
		return nil, errors.New("active business owner email not found")
	}

	return &owner, nil
}

func (s *Service) toPayoutAccountResponse(acct *models.BusinessPayoutAccount) *PayoutAccountResponse {
	return &PayoutAccountResponse{
		ID:            acct.ID,
		BusinessID:    acct.BusinessID,
		BankName:      acct.BankName,
		BankCode:      acct.BankCode,
		AccountNumber: maskAccountNumber(acct.AccountNumber),
		AccountName:   acct.AccountName,
		IsVerified:    acct.IsVerified,
		IsDefault:     acct.IsDefault,
		CreatedAt:     acct.CreatedAt,
		UpdatedAt:     acct.UpdatedAt,
	}
}

func maskAccountNumber(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return value
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}
