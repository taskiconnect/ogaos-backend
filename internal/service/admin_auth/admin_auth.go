// internal/service/admin_auth/admin_auth.go
package admin_auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/crypto"
	"ogaos-backend/internal/pkg/email"
	apperr "ogaos-backend/internal/pkg/errors"
	"ogaos-backend/internal/pkg/jwtpkg"
	"ogaos-backend/internal/pkg/otp"
)

// ── Valid admin roles ─────────────────────────────────────────────────────────

var validAdminRoles = map[string]struct{}{
	"super_admin": {},
	"support":     {},
	"finance":     {},
}

// ── Request / Response types ──────────────────────────────────────────────────

type AdminLoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type AdminLoginResponse struct {
	OTPRequired bool   `json:"otp_required"`
	TempToken   string `json:"temp_token"`
	Message     string `json:"message"`
}

type AdminOTPVerificationRequest struct {
	TempToken string `json:"temp_token" binding:"required"`
	OTP       string `json:"otp"        binding:"required,len=6"`
}

type AdminTokensResponse struct {
	AccessToken string `json:"access_token"`
	Message     string `json:"message"`
}

type AdminPasswordSetupRequest struct {
	Token    string `json:"token"    binding:"required"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

// InviteAdminRequest is sent by a super_admin to create a new platform admin.
type InviteAdminRequest struct {
	Email     string `json:"email"      binding:"required,email"`
	FirstName string `json:"first_name" binding:"required,min=2,max=100"`
	LastName  string `json:"last_name"  binding:"required,min=2,max=100"`
	// Role must be one of: super_admin | support | finance
	Role string `json:"role" binding:"required,oneof=super_admin support finance"`
}

// AdminProfile is returned in list and get responses.
type AdminProfile struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	FirstName   string     `json:"first_name"`
	LastName    string     `json:"last_name"`
	Role        string     `json:"role"`
	IsActive    bool       `json:"is_active"`
	PasswordSet bool       `json:"password_set"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// UpdateAdminRequest allows a super_admin to change another admin's role or status.
type UpdateAdminRequest struct {
	Role     *string `json:"role"      binding:"omitempty,oneof=super_admin support finance"`
	IsActive *bool   `json:"is_active"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type AdminAuthService struct {
	db          *gorm.DB
	jwtSecret   []byte
	accessTTL   time.Duration
	refreshTTL  time.Duration
	frontendURL string
}

func NewAdminAuthService(
	db *gorm.DB,
	adminJWTSecret []byte,
	accessTTL, refreshTTL time.Duration,
	frontendURL string,
) *AdminAuthService {
	return &AdminAuthService{
		db:          db,
		jwtSecret:   adminJWTSecret,
		accessTTL:   accessTTL,
		refreshTTL:  refreshTTL,
		frontendURL: frontendURL,
	}
}

// ── Step 1: Validate credentials → send OTP ───────────────────────────────────

func (s *AdminAuthService) Login(req AdminLoginRequest) (*AdminLoginResponse, error) {
	var admin models.PlatformAdmin
	if err := s.db.Where("LOWER(email) = LOWER(?)", req.Email).First(&admin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Return generic credential error — do not reveal whether the email exists
			return nil, apperr.ErrInvalidCredentials
		}
		return nil, apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	if !admin.IsActive {
		// Use the same generic credential error to avoid account enumeration
		return nil, apperr.ErrInvalidCredentials
	}
	if admin.PasswordSetAt == nil {
		return nil, apperr.New(apperr.CodeUnauthorized, "account setup is not complete — check your email for the setup link")
	}
	ok, err := crypto.VerifyPassword(req.Password, admin.PasswordHash)
	if err != nil || !ok {
		return nil, apperr.ErrInvalidCredentials
	}
	if err := s.incrementRateLimit(admin.ID); err != nil {
		return nil, err
	}
	tempToken, err := s.createTempToken(admin.ID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	if err := s.generateAndSendOTP(admin); err != nil {
		return nil, err
	}
	return &AdminLoginResponse{
		OTPRequired: true,
		TempToken:   tempToken,
		Message:     "OTP sent to your email. Please verify to complete login.",
	}, nil
}

// ── Step 2: Verify OTP → issue tokens ────────────────────────────────────────

func (s *AdminAuthService) VerifyOTP(req AdminOTPVerificationRequest) (*AdminTokensResponse, string, error) {
	adminID, err := s.validateTempToken(req.TempToken)
	if err != nil {
		return nil, "", apperr.ErrInvalidToken
	}
	var admin models.PlatformAdmin
	if err := s.db.First(&admin, "id = ?", adminID).Error; err != nil {
		// Do not distinguish "not found" from other errors here
		return nil, "", apperr.ErrInvalidToken
	}
	if !admin.IsActive {
		return nil, "", apperr.ErrAccountDeactivated
	}

	var adminOTP models.AdminOTP
	if err := s.db.Where("admin_id = ? AND used = false AND expires_at > ?", admin.ID, time.Now()).
		Order("created_at DESC").First(&adminOTP).Error; err != nil {
		return nil, "", apperr.New(apperr.CodeUnauthorized, "no valid OTP found — request a new one")
	}

	result := s.db.Model(&adminOTP).Where("attempts < 5").
		UpdateColumn("attempts", gorm.Expr("attempts + 1"))
	if result.RowsAffected == 0 {
		return nil, "", apperr.New(apperr.CodeUnauthorized, "maximum OTP attempts exceeded — request a new code")
	}
	if !otp.VerifyOTP(req.OTP, adminOTP.OTPHash) {
		return nil, "", apperr.New(apperr.CodeUnauthorized, "invalid OTP code")
	}
	s.db.Model(&adminOTP).UpdateColumn("used", true)
	s.db.Where("admin_id = ?", admin.ID).Delete(&models.AdminOTPRateLimit{})
	s.db.Model(&admin).UpdateColumn("last_login_at", time.Now())

	accessToken, err := jwtpkg.GenerateAdminAccessToken(
		admin.ID, admin.Email, "platform_"+admin.Role, true, s.jwtSecret, s.accessTTL,
	)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	refreshToken, err := s.createRefreshToken(admin.ID)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	return &AdminTokensResponse{AccessToken: accessToken, Message: "Login successful"}, refreshToken, nil
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (s *AdminAuthService) Refresh(rawToken string) (string, string, error) {
	hash := hashToken(rawToken)
	var rt models.AdminRefreshToken
	if err := s.db.Where("token_hash = ? AND expires_at > ? AND revoked = false", hash, time.Now()).
		First(&rt).Error; err != nil {
		return "", "", apperr.ErrInvalidToken
	}
	s.db.Model(&rt).UpdateColumn("revoked", true)

	var admin models.PlatformAdmin
	if err := s.db.First(&admin, "id = ?", rt.AdminID).Error; err != nil {
		return "", "", apperr.ErrInvalidToken
	}
	if !admin.IsActive {
		return "", "", apperr.ErrAccountDeactivated
	}
	accessToken, err := jwtpkg.GenerateAdminAccessToken(
		admin.ID, admin.Email, "platform_"+admin.Role, true, s.jwtSecret, s.accessTTL,
	)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	newRefresh, err := s.createRefreshToken(admin.ID)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	return accessToken, newRefresh, nil
}

// ── Logout ────────────────────────────────────────────────────────────────────

func (s *AdminAuthService) Logout(rawToken string) error {
	hash := hashToken(rawToken)
	return s.db.Model(&models.AdminRefreshToken{}).
		Where("token_hash = ?", hash).
		UpdateColumn("revoked", true).Error
}

// ── Resend OTP ────────────────────────────────────────────────────────────────

func (s *AdminAuthService) ResendOTP(tempToken string) error {
	adminID, err := s.validateTempToken(tempToken)
	if err != nil {
		return apperr.ErrInvalidToken
	}

	var admin models.PlatformAdmin
	if err := s.db.First(&admin, "id = ?", adminID).Error; err != nil {
		return apperr.ErrInvalidToken
	}
	if !admin.IsActive {
		return apperr.ErrAccountDeactivated
	}
	if err := s.generateAndSendOTP(admin); err != nil {
		return err
	}
	return nil
}

// ── Setup password ────────────────────────────────────────────────────────────

func (s *AdminAuthService) SetupPassword(req AdminPasswordSetupRequest) error {
	hash := hashToken(req.Token)
	var admin models.PlatformAdmin
	if err := s.db.Where(
		"password_reset_token = ? AND reset_token_expires > ?", hash, time.Now(),
	).First(&admin).Error; err != nil {
		return apperr.ErrInvalidToken
	}

	hashed, err := crypto.HashPassword(req.Password)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	now := time.Now()
	return s.db.Model(&admin).Updates(map[string]interface{}{
		"password_hash":        hashed,
		"password_set_at":      now,
		"password_reset_token": nil,
		"reset_token_expires":  nil,
	}).Error
}

// ── Admin management ──────────────────────────────────────────────────────────

// InviteAdmin creates a new platform admin record and sends them a setup email.
// The caller must be super_admin — enforced in the route, not here.
func (s *AdminAuthService) InviteAdmin(req InviteAdminRequest) error {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	var count int64
	s.db.Model(&models.PlatformAdmin{}).Where("LOWER(email) = ?", req.Email).Count(&count)
	if count > 0 {
		return apperr.ErrConflict
	}

	token, err := generateSecureToken()
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	tokenHash := hashToken(token)
	expiresAt := time.Now().Add(48 * time.Hour)

	admin := models.PlatformAdmin{
		ID:                 uuid.New(),
		Email:              req.Email,
		FirstName:          strings.TrimSpace(req.FirstName),
		LastName:           strings.TrimSpace(req.LastName),
		PasswordHash:       "UNSET",
		Role:               req.Role,
		IsActive:           true,
		PasswordSetAt:      nil,
		PasswordResetToken: &tokenHash,
		ResetTokenExpires:  &expiresAt,
	}
	if err := s.db.Create(&admin).Error; err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	email.SendAdminPasswordSetupEmail(admin.Email, admin.FirstName, token, s.frontendURL)
	return nil
}

// ListAdmins returns all platform admins.
func (s *AdminAuthService) ListAdmins() ([]AdminProfile, error) {
	var admins []models.PlatformAdmin
	if err := s.db.Order("created_at ASC").Find(&admins).Error; err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	profiles := make([]AdminProfile, len(admins))
	for i, a := range admins {
		profiles[i] = toAdminProfile(a)
	}
	return profiles, nil
}

// GetAdmin returns a single admin by ID.
func (s *AdminAuthService) GetAdmin(adminID uuid.UUID) (*AdminProfile, error) {
	var admin models.PlatformAdmin
	if err := s.db.First(&admin, "id = ?", adminID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	p := toAdminProfile(admin)
	return &p, nil
}

// UpdateAdmin lets a super_admin change another admin's role or active status.
// A super_admin cannot deactivate themselves.
func (s *AdminAuthService) UpdateAdmin(callerID, targetID uuid.UUID, req UpdateAdminRequest) error {
	var target models.PlatformAdmin
	if err := s.db.First(&target, "id = ?", targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.ErrNotFound
		}
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	if req.IsActive != nil && !*req.IsActive && callerID == targetID {
		return apperr.New(apperr.CodeForbidden, "you cannot deactivate your own account")
	}

	updates := make(map[string]interface{})
	if req.Role != nil {
		updates["role"] = *req.Role
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
		if !*req.IsActive {
			s.db.Model(&models.AdminRefreshToken{}).
				Where("admin_id = ?", targetID).
				UpdateColumn("revoked", true)
		}
	}
	if len(updates) == 0 {
		return apperr.New(apperr.CodeBadRequest, "no fields to update")
	}
	if err := s.db.Model(&target).Updates(updates).Error; err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	return nil
}

// DeactivateAdmin is a convenience wrapper for UpdateAdmin that sets is_active=false.
func (s *AdminAuthService) DeactivateAdmin(callerID, targetID uuid.UUID) error {
	f := false
	return s.UpdateAdmin(callerID, targetID, UpdateAdminRequest{IsActive: &f})
}

// WhoAmI returns the profile of the currently authenticated admin.
func (s *AdminAuthService) WhoAmI(adminID uuid.UUID) (*AdminProfile, error) {
	return s.GetAdmin(adminID)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func toAdminProfile(a models.PlatformAdmin) AdminProfile {
	return AdminProfile{
		ID:          a.ID,
		Email:       a.Email,
		FirstName:   a.FirstName,
		LastName:    a.LastName,
		Role:        a.Role,
		IsActive:    a.IsActive,
		PasswordSet: a.PasswordSetAt != nil,
		LastLoginAt: a.LastLoginAt,
		CreatedAt:   a.CreatedAt,
	}
}

func (s *AdminAuthService) generateAndSendOTP(admin models.PlatformAdmin) error {
	otpCode, err := otp.GenerateOTP()
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("admin_id = ? AND used = false", admin.ID).Delete(&models.AdminOTP{})
		if err := tx.Create(&models.AdminOTP{
			AdminID:   admin.ID,
			OTPHash:   otp.HashOTP(otpCode),
			ExpiresAt: time.Now().Add(5 * time.Minute),
		}).Error; err != nil {
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
		email.SendAdminOTPEmail(admin.Email, admin.FirstName, otpCode)
		return nil
	})
}

func (s *AdminAuthService) incrementRateLimit(adminID uuid.UUID) error {
	s.db.Exec(`
		UPDATE admin_otp_rate_limits
		SET attempts = 0, last_attempt_at = NOW()
		WHERE admin_id = ? AND last_attempt_at < NOW() - INTERVAL '15 minutes'
	`, adminID)

	var rl models.AdminOTPRateLimit
	err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "admin_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"attempts":        gorm.Expr("admin_otp_rate_limits.attempts + 1"),
			"last_attempt_at": gorm.Expr("NOW()"),
		}),
	}).Create(&models.AdminOTPRateLimit{
		AdminID:       adminID,
		Attempts:      1,
		LastAttemptAt: time.Now(),
	}).Error
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	if err := s.db.Where("admin_id = ?", adminID).First(&rl).Error; err != nil {
		return nil
	}
	if rl.Attempts > 5 {
		return apperr.New(apperr.CodeUnauthorized, "too many attempts — please wait 15 minutes before trying again")
	}
	return nil
}

func (s *AdminAuthService) createTempToken(adminID uuid.UUID) (string, error) {
	claims := jwt.MapClaims{
		"sub":  adminID.String(),
		"exp":  time.Now().Add(10 * time.Minute).Unix(),
		"iat":  time.Now().Unix(),
		"iss":  "ogaos-admin-otp",
		"type": "admin_otp_session",
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *AdminAuthService) validateTempToken(tokenStr string) (uuid.UUID, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return uuid.Nil, apperr.ErrInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, apperr.ErrInvalidToken
	}
	if claims["iss"] != "ogaos-admin-otp" || claims["type"] != "admin_otp_session" {
		return uuid.Nil, apperr.ErrInvalidToken
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return uuid.Nil, apperr.ErrInvalidToken
	}
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, apperr.ErrInvalidToken
	}
	return id, nil
}

func (s *AdminAuthService) createRefreshToken(adminID uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	hash := hashToken(raw)
	if err := s.db.Create(&models.AdminRefreshToken{
		TokenHash: hash,
		AdminID:   adminID,
		ExpiresAt: time.Now().Add(s.refreshTTL),
	}).Error; err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	return raw, nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(h[:])
}

func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
