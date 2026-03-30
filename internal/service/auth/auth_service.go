// internal/service/auth/auth_service.go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
	"ogaos-backend/internal/pkg/crypto"
	"ogaos-backend/internal/pkg/email"
	apperr "ogaos-backend/internal/pkg/errors"
	"ogaos-backend/internal/pkg/jwtpkg"
)

// ── Request / Response types ──────────────────────────────────────────────────

// RegisterRequest uses binding tags so ShouldBindJSON enforces all constraints.
type RegisterRequest struct {
	FirstName        string `json:"first_name"        binding:"required,min=2,max=100"`
	LastName         string `json:"last_name"         binding:"required,min=2,max=100"`
	PhoneNumber      string `json:"phone_number"      binding:"required,min=7,max=20"`
	Email            string `json:"email"             binding:"required,email"`
	Password         string `json:"password"          binding:"required,min=8,max=72"`
	BusinessName     string `json:"business_name"     binding:"required,min=2,max=255"`
	BusinessCategory string `json:"business_category" binding:"required,min=2,max=100"`
	Street           string `json:"street"            binding:"omitempty,max=255"`
	CityTown         string `json:"city_town"         binding:"omitempty,max=100"`
	LocalGovernment  string `json:"local_government"  binding:"omitempty,max=100"`
	State            string `json:"state"             binding:"omitempty,max=100"`
	Country          string `json:"country"           binding:"omitempty,max=100"`
	ReferralCode     string `json:"referral_code"     binding:"omitempty,max=50"`
}

// StaffCreateRequest binds staff creation payload.
type StaffCreateRequest struct {
	FirstName   string `json:"first_name"   binding:"required,min=2,max=100"`
	LastName    string `json:"last_name"    binding:"required,min=2,max=100"`
	PhoneNumber string `json:"phone_number" binding:"required,min=7,max=20"`
	Email       string `json:"email"        binding:"required,email"`
	Password    string `json:"password"     binding:"required,min=8,max=72"`
}

// BusinessProfile is a lightweight summary embedded in WhoAmIResponse.
type BusinessProfile struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Category        string    `json:"category"`
	Status          string    `json:"status"`
	Street          string    `json:"street"`
	CityTown        string    `json:"city_town"`
	LocalGovernment string    `json:"local_government"`
	State           string    `json:"state"`
	Country         string    `json:"country"`
}

// WhoAmIResponse is the payload returned by the /auth/me endpoint.
type WhoAmIResponse struct {
	ID              uuid.UUID        `json:"id"`
	FirstName       string           `json:"first_name"`
	LastName        string           `json:"last_name"`
	Email           string           `json:"email"`
	PhoneNumber     string           `json:"phone_number"`
	EmailVerifiedAt *time.Time       `json:"email_verified_at"`
	IsActive        bool             `json:"is_active"`
	Role            string           `json:"role"`
	IsPlatform      bool             `json:"is_platform"`
	CreatedAt       time.Time        `json:"created_at"`
	Business        *BusinessProfile `json:"business,omitempty"`
}

// StaffMember is used in list-staff responses.
type StaffMember struct {
	ID          uuid.UUID `json:"id"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	Email       string    `json:"email"`
	PhoneNumber string    `json:"phone_number"`
	Role        string    `json:"role"`
	IsActive    bool      `json:"is_active"`
	JoinedAt    time.Time `json:"joined_at"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type AuthService struct {
	db          *gorm.DB
	jwtSecret   []byte
	accessTTL   time.Duration
	refreshTTL  time.Duration
	frontendURL string
}

func NewAuthService(
	db *gorm.DB,
	jwtSecret []byte,
	accessTTL, refreshTTL time.Duration,
	frontendURL string,
) *AuthService {
	return &AuthService{
		db:          db,
		jwtSecret:   jwtSecret,
		accessTTL:   accessTTL,
		refreshTTL:  refreshTTL,
		frontendURL: frontendURL,
	}
}

// ── Register ──────────────────────────────────────────────────────────────────

// Register handles two paths:
//
//	A) Brand-new person  → creates User + Business + BusinessUser(owner)
//	B) Existing user registering a second business → verifies password, creates Business + BusinessUser(owner)
func (s *AuthService) Register(req RegisterRequest) error {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)

	var existing models.User
	foundByEmail := s.db.Where("LOWER(email) = ?", req.Email).First(&existing).Error == nil
	foundByPhone := !foundByEmail && s.db.Where("phone_number = ?", req.PhoneNumber).First(&existing).Error == nil

	if foundByEmail || foundByPhone {
		ok, err := crypto.VerifyPassword(req.Password, existing.PasswordHash)
		if err != nil || !ok {
			// Generic message — do not confirm whether email/phone is registered
			return apperr.New(apperr.CodeConflict, "an account with this email or phone already exists — use your existing password to register the business")
		}
		return s.db.Transaction(func(tx *gorm.DB) error {
			business := s.buildBusiness(req)
			business.Slug = s.uniqueSlug(tx, req.BusinessName)
			if err := tx.Create(&business).Error; err != nil {
				return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
			}
			tx.Model(&models.BusinessUser{}).
				Where("user_id = ? AND role = 'staff'", existing.ID).
				Update("is_active", false)
			if err := tx.Create(&models.BusinessUser{
				BusinessID: business.ID, UserID: existing.ID, Role: "owner",
			}).Error; err != nil {
				return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
			}
			return nil
		})
	}

	hashed, err := crypto.HashPassword(req.Password)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	rawToken, tokenHash, err := generateVerificationToken()
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	expiresAt := time.Now().Add(48 * time.Hour)
	user := models.User{
		FirstName:             req.FirstName,
		LastName:              req.LastName,
		Email:                 req.Email,
		PhoneNumber:           req.PhoneNumber,
		PasswordHash:          hashed,
		VerificationToken:     &tokenHash,
		VerificationExpiresAt: &expiresAt,
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
		business := s.buildBusiness(req)
		business.Slug = s.uniqueSlug(tx, req.BusinessName)
		if err := tx.Create(&business).Error; err != nil {
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
		if err := tx.Create(&models.BusinessUser{
			BusinessID: business.ID, UserID: user.ID, Role: "owner",
		}).Error; err != nil {
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
		email.SendVerificationEmail(user.Email, rawToken, s.frontendURL, business.Name)
		return nil
	})
}

// ── Email verification ────────────────────────────────────────────────────────

// VerifyEmail matches the hashed token stored in the DB.
// The raw token arrives via query string; we hash it before lookup.
func (s *AuthService) VerifyEmail(rawToken string) error {
	tokenHash := hashVerificationToken(rawToken)
	var user models.User
	if err := s.db.Where(
		"verification_token = ? AND verification_expires_at > NOW()", tokenHash,
	).First(&user).Error; err != nil {
		return apperr.ErrInvalidToken
	}
	now := time.Now()
	if err := s.db.Model(&user).Updates(map[string]interface{}{
		"email_verified_at":       now,
		"verification_token":      nil,
		"verification_expires_at": nil,
	}).Error; err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	return nil
}

// ResendVerification re-issues a verification email.
// Always returns nil — never leaks whether the email exists.
func (s *AuthService) ResendVerification(emailAddr string) error {
	emailAddr = strings.ToLower(strings.TrimSpace(emailAddr))

	var user models.User
	if err := s.db.Where("LOWER(email) = ?", emailAddr).First(&user).Error; err != nil {
		return nil // silent — no enumeration
	}
	if user.EmailVerifiedAt != nil {
		return nil // already verified — stay silent
	}

	rawToken, tokenHash, err := generateVerificationToken()
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	expiresAt := time.Now().Add(48 * time.Hour)

	if err := s.db.Model(&user).Updates(map[string]interface{}{
		"verification_token":      tokenHash,
		"verification_expires_at": expiresAt,
	}).Error; err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	var businessName string
	var bu models.BusinessUser
	if s.db.Where("user_id = ?", user.ID).First(&bu).Error == nil {
		var biz models.Business
		if s.db.First(&biz, bu.BusinessID).Error == nil {
			businessName = biz.Name
		}
	}
	email.SendVerificationEmail(user.Email, rawToken, s.frontendURL, businessName)
	return nil
}

// ── Login ─────────────────────────────────────────────────────────────────────

// Login handles regular user authentication only.
func (s *AuthService) Login(emailAddr, password string) (accessToken, refreshToken string, err error) {
	emailAddr = strings.ToLower(strings.TrimSpace(emailAddr))

	type loginRow struct {
		UserID       string
		Email        string
		PasswordHash string
		IsActive     bool
		VerifiedAt   *time.Time
		BusinessID   string
		Role         string
		BUActive     bool
	}

	var row loginRow
	queryErr := s.db.Raw(`
		SELECT
			u.id          AS user_id,
			u.email,
			u.password_hash,
			u.is_active,
			u.email_verified_at AS verified_at,
			bu.business_id,
			bu.role,
			bu.is_active  AS bu_active
		FROM users u
		LEFT JOIN business_users bu
			ON bu.user_id = u.id AND bu.is_active = true
		WHERE LOWER(u.email) = ?
		ORDER BY bu.created_at DESC
		LIMIT 1
	`, emailAddr).Scan(&row).Error

	if queryErr != nil || row.UserID == "" {
		// Generic — do not reveal whether the email exists
		return "", "", apperr.ErrInvalidCredentials
	}
	if row.VerifiedAt == nil {
		return "", "", apperr.New(apperr.CodeUnauthorized, "email not verified — check your inbox")
	}
	if !row.IsActive {
		// Generic credential error to avoid account enumeration
		return "", "", apperr.ErrInvalidCredentials
	}

	ok, verifyErr := crypto.VerifyPassword(password, row.PasswordHash)
	if verifyErr != nil || !ok {
		return "", "", apperr.ErrInvalidCredentials
	}

	if !row.BUActive || row.BusinessID == "" {
		return "", "", apperr.New(apperr.CodeForbidden, "no active business role found — contact your administrator")
	}

	userID := uuid.MustParse(row.UserID)
	bizID := uuid.MustParse(row.BusinessID)

	accessToken, err = jwtpkg.GenerateAccessToken(
		userID, row.Email, bizID, row.Role, false, s.jwtSecret, s.accessTTL,
	)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	refreshToken, err = s.createRefreshToken(userID)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	return accessToken, refreshToken, nil
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (s *AuthService) Refresh(rawRefresh string) (newAccess, newRefresh string, err error) {
	hash := hashRefreshToken(rawRefresh)

	var rt models.RefreshToken
	if err = s.db.Where("token_hash = ? AND expires_at > ? AND revoked = false", hash, time.Now()).
		First(&rt).Error; err != nil {
		return "", "", apperr.ErrInvalidToken
	}

	// Rotate: revoke old token immediately
	s.db.Model(&rt).UpdateColumn("revoked", true)

	type refreshRow struct {
		Email      string
		IsActive   bool
		BusinessID string
		Role       string
		BUActive   bool
	}
	var row refreshRow
	s.db.Raw(`
		SELECT u.email, u.is_active, bu.business_id, bu.role, bu.is_active AS bu_active
		FROM users u
		LEFT JOIN business_users bu ON bu.user_id = u.id AND bu.is_active = true
		WHERE u.id = ?
		ORDER BY bu.created_at DESC
		LIMIT 1
	`, rt.UserID).Scan(&row)

	if !row.IsActive {
		return "", "", apperr.ErrAccountDeactivated
	}

	bizID := uuid.Nil
	if row.BusinessID != "" {
		bizID = uuid.MustParse(row.BusinessID)
	}

	newAccess, err = jwtpkg.GenerateAccessToken(
		rt.UserID, row.Email, bizID, row.Role, false, s.jwtSecret, s.accessTTL,
	)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	newRefresh, err = s.createRefreshToken(rt.UserID)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	return newAccess, newRefresh, nil
}

// ── Logout ────────────────────────────────────────────────────────────────────

func (s *AuthService) Logout(rawRefresh string) error {
	hash := hashRefreshToken(rawRefresh)
	return s.db.Model(&models.RefreshToken{}).
		Where("token_hash = ?", hash).
		UpdateColumn("revoked", true).Error
}

// ── WhoAmI ────────────────────────────────────────────────────────────────────

// WhoAmI fetches user + business info in a single JOIN query.
func (s *AuthService) WhoAmI(userID uuid.UUID, isPlatform bool) (*WhoAmIResponse, error) {
	if isPlatform {
		return nil, apperr.ErrForbidden
	}

	type whoRow struct {
		ID              string
		FirstName       string
		LastName        string
		Email           string
		PhoneNumber     string
		EmailVerifiedAt *time.Time
		IsActive        bool
		CreatedAt       time.Time
		Role            string
		BusinessID      string
		BizName         string
		BizCategory     string
		BizStatus       string
		BizStreet       string
		BizCity         string
		BizLGA          string
		BizState        string
		BizCountry      string
	}

	var row whoRow
	err := s.db.Raw(`
		SELECT
			u.id, u.first_name, u.last_name, u.email, u.phone_number,
			u.email_verified_at, u.is_active, u.created_at,
			bu.role,
			b.id   AS business_id,
			b.name AS biz_name, b.category AS biz_category, b.status AS biz_status,
			b.street AS biz_street, b.city_town AS biz_city,
			b.local_government AS biz_lga, b.state AS biz_state, b.country AS biz_country
		FROM users u
		LEFT JOIN business_users bu ON bu.user_id = u.id AND bu.is_active = true
		LEFT JOIN businesses b ON b.id = bu.business_id
		WHERE u.id = ?
		ORDER BY bu.created_at DESC
		LIMIT 1
	`, userID).Scan(&row).Error
	if err != nil || row.ID == "" {
		return nil, apperr.ErrNotFound
	}

	resp := &WhoAmIResponse{
		ID:              uuid.MustParse(row.ID),
		FirstName:       row.FirstName,
		LastName:        row.LastName,
		Email:           row.Email,
		PhoneNumber:     row.PhoneNumber,
		EmailVerifiedAt: row.EmailVerifiedAt,
		IsActive:        row.IsActive,
		Role:            row.Role,
		IsPlatform:      false,
		CreatedAt:       row.CreatedAt,
	}

	if row.BusinessID != "" {
		resp.Business = &BusinessProfile{
			ID:              uuid.MustParse(row.BusinessID),
			Name:            row.BizName,
			Category:        row.BizCategory,
			Status:          row.BizStatus,
			Street:          row.BizStreet,
			CityTown:        row.BizCity,
			LocalGovernment: row.BizLGA,
			State:           row.BizState,
			Country:         row.BizCountry,
		}
	}
	return resp, nil
}

// ── Staff ─────────────────────────────────────────────────────────────────────

// CreateStaff adds a staff member to a business.
func (s *AuthService) CreateStaff(businessID uuid.UUID, req StaffCreateRequest) error {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)

	var business models.Business
	if err := s.db.First(&business, "id = ?", businessID).Error; err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	var existing models.User
	foundByEmail := s.db.Where("LOWER(email) = ?", req.Email).First(&existing).Error == nil
	if !foundByEmail {
		s.db.Where("phone_number = ?", req.PhoneNumber).First(&existing)
	}

	if existing.ID != uuid.Nil {
		var link models.BusinessUser
		if s.db.Where("business_id = ? AND user_id = ?", businessID, existing.ID).First(&link).Error == nil {
			if link.IsActive {
				return apperr.New(apperr.CodeConflict, "this person is already a member of your business")
			}
			if err := s.db.Model(&link).UpdateColumn("is_active", true).Error; err != nil {
				return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
			}
			email.SendStaffInvitationEmail(existing.Email, "", s.frontendURL, business.Name)
			return nil
		}
		if err := s.db.Create(&models.BusinessUser{
			BusinessID: businessID, UserID: existing.ID, Role: "staff",
		}).Error; err != nil {
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
		email.SendStaffInvitationEmail(existing.Email, "", s.frontendURL, business.Name)
		return nil
	}

	hashed, err := crypto.HashPassword(req.Password)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	rawToken, tokenHash, err := generateVerificationToken()
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}

	expires := time.Now().Add(48 * time.Hour)
	staff := models.User{
		FirstName:             req.FirstName,
		LastName:              req.LastName,
		Email:                 req.Email,
		PhoneNumber:           req.PhoneNumber,
		PasswordHash:          hashed,
		VerificationToken:     &tokenHash,
		VerificationExpiresAt: &expires,
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&staff).Error; err != nil {
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
		if err := tx.Create(&models.BusinessUser{
			BusinessID: businessID, UserID: staff.ID, Role: "staff",
		}).Error; err != nil {
			return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
		}
		email.SendStaffInvitationEmail(req.Email, rawToken, s.frontendURL, business.Name)
		return nil
	})
}

func (s *AuthService) DeactivateStaff(businessID, staffUserID uuid.UUID) error {
	var link models.BusinessUser
	if err := s.db.Where(
		"business_id = ? AND user_id = ? AND role = 'staff'", businessID, staffUserID,
	).First(&link).Error; err != nil {
		return apperr.ErrNotFound
	}
	if !link.IsActive {
		return apperr.New(apperr.CodeBadRequest, "staff member is already deactivated")
	}
	if err := s.db.Model(&link).UpdateColumn("is_active", false).Error; err != nil {
		return apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	return nil
}

func (s *AuthService) ListStaff(businessID uuid.UUID) ([]StaffMember, error) {
	var rows []struct {
		ID          string
		FirstName   string
		LastName    string
		Email       string
		PhoneNumber string
		Role        string
		IsActive    bool
		JoinedAt    time.Time
	}
	err := s.db.Raw(`
		SELECT u.id, u.first_name, u.last_name, u.email, u.phone_number,
		       bu.role, bu.is_active, bu.created_at AS joined_at
		FROM business_users bu
		JOIN users u ON u.id = bu.user_id
		WHERE bu.business_id = ? AND bu.role = 'staff'
		ORDER BY bu.created_at DESC
	`, businessID).Scan(&rows).Error
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	members := make([]StaffMember, len(rows))
	for i, r := range rows {
		members[i] = StaffMember{
			ID:          uuid.MustParse(r.ID),
			FirstName:   r.FirstName,
			LastName:    r.LastName,
			Email:       r.Email,
			PhoneNumber: r.PhoneNumber,
			Role:        r.Role,
			IsActive:    r.IsActive,
			JoinedAt:    r.JoinedAt,
		}
	}
	return members, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// createRefreshToken stores a hashed token and returns the raw value.
func (s *AuthService) createRefreshToken(userID uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	hash := hashRefreshToken(raw)

	if err := s.db.Create(&models.RefreshToken{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: time.Now().Add(s.refreshTTL),
	}).Error; err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, apperr.ErrInternal.Message, err)
	}
	return raw, nil
}

// generateVerificationToken creates a CSPRNG token and returns (rawToken, hash).
// The raw token is emailed; only the hash is stored in the DB.
func generateVerificationToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand.Read: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	hash = hashVerificationToken(raw)
	return raw, hash, nil
}

// hashVerificationToken produces the stored digest for a verification token.
func hashVerificationToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return base64.StdEncoding.EncodeToString(h[:])
}

// hashRefreshToken produces the stored digest for a refresh token.
func hashRefreshToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return base64.RawStdEncoding.EncodeToString(h[:])
}

// ── Slug helpers ──────────────────────────────────────────────────────────────

func generateSlug(name string) string {
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, name)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return strings.Trim(slug, "-")
}

// uniqueSlug does a single pre-check query to find existing slugs with the same base,
// then picks a collision-free variant — no looping on individual lookups.
func (s *AuthService) uniqueSlug(tx *gorm.DB, name string) string {
	base := generateSlug(name)

	var existing []string
	tx.Model(&models.Business{}).
		Where("slug LIKE ?", base+"%").
		Pluck("slug", &existing)

	taken := make(map[string]struct{}, len(existing))
	for _, s := range existing {
		taken[s] = struct{}{}
	}

	if _, used := taken[base]; !used {
		return base
	}

	for i := 0; i < 20; i++ {
		b := make([]byte, 3)
		rand.Read(b)
		candidate := fmt.Sprintf("%s-%s", base, base64.RawURLEncoding.EncodeToString(b)[:4])
		if _, used := taken[candidate]; !used {
			return candidate
		}
	}
	// Fallback: use UUID suffix — guaranteed unique
	return fmt.Sprintf("%s-%s", base, uuid.NewString()[:8])
}

// buildBusiness constructs a Business model from a RegisterRequest.
func (s *AuthService) buildBusiness(req RegisterRequest) models.Business {
	return models.Business{
		Name:             req.BusinessName,
		Category:         req.BusinessCategory,
		Street:           req.Street,
		CityTown:         req.CityTown,
		LocalGovernment:  req.LocalGovernment,
		State:            req.State,
		Country:          req.Country,
		ReferralCodeUsed: req.ReferralCode,
	}
}

// keep errors imported to avoid "imported and not used" in case of future use
var _ = errors.New
