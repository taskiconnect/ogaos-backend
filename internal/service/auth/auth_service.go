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
	"ogaos-backend/internal/pkg/jwtpkg"
)

// generateSlug converts a business name into a lowercase URL-safe slug.
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

// uniqueSlug returns a collision-free slug by appending a random suffix if needed.
func (s *AuthService) uniqueSlug(tx *gorm.DB, name string) string {
	base := generateSlug(name)
	slug := base
	for i := 0; i < 10; i++ {
		var count int64
		tx.Model(&models.Business{}).Where("slug = ?", slug).Count(&count)
		if count == 0 {
			return slug
		}
		b := make([]byte, 3)
		rand.Read(b)
		slug = fmt.Sprintf("%s-%s", base, base64.RawURLEncoding.EncodeToString(b)[:4])
	}
	return slug
}

type RegisterRequest struct {
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name"`
	PhoneNumber      string `json:"phone_number"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	BusinessName     string `json:"business_name"`
	BusinessCategory string `json:"business_category"`
	Street           string `json:"street,omitempty"`
	CityTown         string `json:"city_town,omitempty"`
	LocalGovernment  string `json:"local_government,omitempty"`
	State            string `json:"state,omitempty"`
	Country          string `json:"country,omitempty"`
	ReferralCode     string `json:"referral_code,omitempty"`
}

type StaffCreateRequest struct {
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	PhoneNumber string `json:"phone_number"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

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

type AuthService struct {
	db          *gorm.DB
	jwtSecret   []byte
	accessTTL   time.Duration
	refreshTTL  time.Duration
	frontendURL string
}

func NewAuthService(db *gorm.DB, jwtSecret []byte, accessTTL, refreshTTL time.Duration, frontendURL string) *AuthService {
	return &AuthService{db: db, jwtSecret: jwtSecret, accessTTL: accessTTL, refreshTTL: refreshTTL, frontendURL: frontendURL}
}

type Handler struct{ service *AuthService }

func NewHandler(service *AuthService) *Handler { return &Handler{service: service} }

// Register handles two scenarios:
// A) New person: create User + Business + BusinessUser(owner)
// B) Existing staff/owner registering a new business: verify password, create Business + BusinessUser(owner)
func (s *AuthService) Register(req RegisterRequest) error {
	var existingUser models.User
	found := false

	if s.db.Where("email = ?", req.Email).First(&existingUser).Error == nil {
		found = true
	} else if s.db.Where("phone_number = ?", req.PhoneNumber).First(&existingUser).Error == nil {
		found = true
	}

	if found {
		// Verify they own this account with their password
		if ok, _ := crypto.VerifyPassword(req.Password, existingUser.PasswordHash); !ok {
			return errors.New("an account with this email or phone already exists — use your existing password to register your business")
		}

		// Same person — create a new business and make them owner
		return s.db.Transaction(func(tx *gorm.DB) error {
			business := models.Business{
				Name: req.BusinessName, Slug: s.uniqueSlug(tx, req.BusinessName),
				Category: req.BusinessCategory,
				Street:   req.Street, CityTown: req.CityTown,
				LocalGovernment: req.LocalGovernment, State: req.State,
				Country: req.Country, ReferralCodeUsed: req.ReferralCode,
			}
			if err := tx.Create(&business).Error; err != nil {
				return err
			}
			// Deactivate old staff role so login picks owner role
			tx.Model(&models.BusinessUser{}).
				Where("user_id = ? AND role = 'staff'", existingUser.ID).
				Update("is_active", false)
			bu := models.BusinessUser{BusinessID: business.ID, UserID: existingUser.ID, Role: "owner"}
			return tx.Create(&bu).Error
		})
	}

	// New person
	hashed, err := crypto.HashPassword(req.Password)
	if err != nil {
		return err
	}
	token := uuid.NewString()
	expiresAt := time.Now().Add(48 * time.Hour)
	user := models.User{
		FirstName: req.FirstName, LastName: req.LastName,
		Email: req.Email, PhoneNumber: req.PhoneNumber,
		PasswordHash: hashed, VerificationToken: &token, VerificationExpiresAt: &expiresAt,
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		business := models.Business{
			Name: req.BusinessName, Slug: s.uniqueSlug(tx, req.BusinessName),
			Category: req.BusinessCategory,
			Street:   req.Street, CityTown: req.CityTown,
			LocalGovernment: req.LocalGovernment, State: req.State,
			Country: req.Country, ReferralCodeUsed: req.ReferralCode,
		}
		if err := tx.Create(&business).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.BusinessUser{BusinessID: business.ID, UserID: user.ID, Role: "owner"}).Error; err != nil {
			return err
		}
		email.SendVerificationEmail(user.Email, token, s.frontendURL, business.Name)
		return nil
	})
}

func (s *AuthService) VerifyEmail(token string) error {
	var user models.User
	if err := s.db.Where("verification_token = ? AND verification_expires_at > NOW()", token).First(&user).Error; err != nil {
		return errors.New("invalid or expired verification token")
	}
	now := time.Now()
	return s.db.Model(&user).Updates(map[string]interface{}{
		"email_verified_at": now, "verification_token": nil, "verification_expires_at": nil,
	}).Error
}

func (s *AuthService) Login(emailAddr, password string) (accessToken, refreshToken string, err error) {
	var user models.User
	if err = s.db.Where("email = ?", emailAddr).First(&user).Error; err == nil {
		if user.EmailVerifiedAt == nil {
			return "", "", errors.New("email not verified — check your inbox")
		}
		if !user.IsActive {
			return "", "", errors.New("account deactivated")
		}
		if ok, _ := crypto.VerifyPassword(password, user.PasswordHash); !ok {
			return "", "", errors.New("invalid credentials")
		}
		var bu models.BusinessUser
		if err = s.db.Where("user_id = ? AND is_active = true", user.ID).Order("created_at DESC").First(&bu).Error; err != nil {
			return "", "", errors.New("no active business role found")
		}
		accessToken, _ = jwtpkg.GenerateAccessToken(user.ID, user.Email, bu.BusinessID, bu.Role, false, s.jwtSecret, s.accessTTL)
		refreshToken = s.createRefreshToken(user.ID)
		return accessToken, refreshToken, nil
	}

	var admin models.PlatformAdmin
	if err = s.db.Where("email = ?", emailAddr).First(&admin).Error; err == nil {
		if !admin.IsActive {
			return "", "", errors.New("account deactivated")
		}
		if ok, _ := crypto.VerifyPassword(password, admin.PasswordHash); !ok {
			return "", "", errors.New("invalid credentials")
		}
		accessToken, _ = jwtpkg.GenerateAccessToken(admin.ID, admin.Email, uuid.Nil, "platform_"+admin.Role, true, s.jwtSecret, s.accessTTL)
		refreshToken = s.createRefreshToken(admin.ID)
		return accessToken, refreshToken, nil
	}
	return "", "", errors.New("invalid credentials")
}

func (s *AuthService) createRefreshToken(userID uuid.UUID) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	hashStr := base64.RawStdEncoding.EncodeToString(hash[:])
	s.db.Create(&models.RefreshToken{TokenHash: hashStr, UserID: userID, ExpiresAt: time.Now().Add(s.refreshTTL)})
	return token
}

func (s *AuthService) Refresh(refreshToken string) (newAccess string, newRefresh string, err error) {
	hashBytes := sha256.Sum256([]byte(refreshToken))
	hash := base64.RawStdEncoding.EncodeToString(hashBytes[:])
	var rt models.RefreshToken
	if err = s.db.Where("token_hash = ? AND expires_at > ? AND revoked = false", hash, time.Now()).First(&rt).Error; err != nil {
		return "", "", errors.New("invalid or expired refresh token")
	}
	s.db.Model(&rt).Update("revoked", true)
	var user models.User
	if err = s.db.First(&user, rt.UserID).Error; err == nil {
		var bu models.BusinessUser
		s.db.Where("user_id = ? AND is_active = true", user.ID).Order("created_at DESC").First(&bu)
		newAccess, _ = jwtpkg.GenerateAccessToken(user.ID, user.Email, bu.BusinessID, bu.Role, false, s.jwtSecret, s.accessTTL)
	} else {
		var admin models.PlatformAdmin
		if err = s.db.First(&admin, rt.UserID).Error; err == nil {
			newAccess, _ = jwtpkg.GenerateAccessToken(admin.ID, admin.Email, uuid.Nil, "platform_"+admin.Role, true, s.jwtSecret, s.accessTTL)
		} else {
			return "", "", errors.New("user not found")
		}
	}
	newRefresh = s.createRefreshToken(rt.UserID)
	return newAccess, newRefresh, nil
}

func (s *AuthService) Logout(refreshToken string) error {
	hashBytes := sha256.Sum256([]byte(refreshToken))
	hash := base64.RawStdEncoding.EncodeToString(hashBytes[:])
	return s.db.Model(&models.RefreshToken{}).Where("token_hash = ?", hash).Update("revoked", true).Error
}

func (s *AuthService) WhoAmI(userID uuid.UUID, isPlatform bool) (*WhoAmIResponse, error) {
	if isPlatform {
		var admin models.PlatformAdmin
		if err := s.db.First(&admin, userID).Error; err != nil {
			return nil, errors.New("platform admin not found")
		}
		return &WhoAmIResponse{ID: admin.ID, FirstName: admin.FirstName, LastName: admin.LastName,
			Email: admin.Email, IsActive: admin.IsActive, Role: "platform_" + admin.Role,
			IsPlatform: true, CreatedAt: admin.CreatedAt}, nil
	}
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, errors.New("user not found")
	}
	resp := &WhoAmIResponse{
		ID: user.ID, FirstName: user.FirstName, LastName: user.LastName,
		Email: user.Email, PhoneNumber: user.PhoneNumber,
		EmailVerifiedAt: user.EmailVerifiedAt, IsActive: user.IsActive,
		IsPlatform: false, CreatedAt: user.CreatedAt,
	}
	var bu models.BusinessUser
	if err := s.db.Where("user_id = ? AND is_active = true", userID).Order("created_at DESC").First(&bu).Error; err == nil {
		resp.Role = bu.Role
		var business models.Business
		if err := s.db.First(&business, bu.BusinessID).Error; err == nil {
			resp.Business = &BusinessProfile{
				ID: business.ID, Name: business.Name, Category: business.Category,
				Status: business.Status, Street: business.Street, CityTown: business.CityTown,
				LocalGovernment: business.LocalGovernment, State: business.State, Country: business.Country,
			}
		}
	}
	return resp, nil
}

// CreateStaff handles two scenarios:
// A) New person: create User + BusinessUser(staff)
// B) Existing user: just add BusinessUser(staff) link — no new User created
func (s *AuthService) CreateStaff(businessID uuid.UUID, req StaffCreateRequest) error {
	var count int64
	s.db.Model(&models.BusinessUser{}).
		Where("business_id = ? AND role = 'staff' AND is_active = true", businessID).
		Count(&count)
	if count >= 2 {
		return errors.New("maximum 2 staff allowed on Starter plan")
	}

	var business models.Business
	s.db.First(&business, businessID)

	// Check if user already exists
	var existingUser models.User
	found := s.db.Where("email = ?", req.Email).First(&existingUser).Error == nil
	if !found {
		found = s.db.Where("phone_number = ?", req.PhoneNumber).First(&existingUser).Error == nil
	}

	if found {
		// Check if already linked to this business
		var existing models.BusinessUser
		if s.db.Where("business_id = ? AND user_id = ?", businessID, existingUser.ID).First(&existing).Error == nil {
			if existing.IsActive {
				return errors.New("this person is already a member of your business")
			}
			// Re-activate
			return s.db.Model(&existing).Update("is_active", true).Error
		}
		// Link existing user as staff
		if err := s.db.Create(&models.BusinessUser{BusinessID: businessID, UserID: existingUser.ID, Role: "staff"}).Error; err != nil {
			return err
		}
		email.SendStaffInvitationEmail(existingUser.Email, "", s.frontendURL, business.Name)
		return nil
	}

	// New user
	hashed, _ := crypto.HashPassword(req.Password)
	token := uuid.NewString()
	expires := time.Now().Add(48 * time.Hour)
	staff := models.User{
		FirstName: req.FirstName, LastName: req.LastName,
		Email: req.Email, PhoneNumber: req.PhoneNumber,
		PasswordHash: hashed, VerificationToken: &token, VerificationExpiresAt: &expires,
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&staff).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.BusinessUser{BusinessID: businessID, UserID: staff.ID, Role: "staff"}).Error; err != nil {
			return err
		}
		email.SendStaffInvitationEmail(req.Email, token, s.frontendURL, business.Name)
		return nil
	})
}

func (s *AuthService) DeactivateStaff(businessID uuid.UUID, staffUserID uuid.UUID) error {
	var link models.BusinessUser
	if err := s.db.Where("business_id = ? AND user_id = ? AND role = 'staff'", businessID, staffUserID).First(&link).Error; err != nil {
		return errors.New("staff member not found in this business")
	}
	if !link.IsActive {
		return errors.New("staff already deactivated")
	}
	return s.db.Model(&link).Update("is_active", false).Error
}

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

func (s *AuthService) ListStaff(businessID uuid.UUID) ([]StaffMember, error) {
	type row struct {
		ID          uuid.UUID
		FirstName   string
		LastName    string
		Email       string
		PhoneNumber string
		Role        string
		IsActive    bool
		JoinedAt    time.Time
	}
	var rows []row
	err := s.db.Table("business_users bu").
		Select("u.id, u.first_name, u.last_name, u.email, u.phone_number, bu.role, bu.is_active, bu.created_at as joined_at").
		Joins("JOIN users u ON u.id = bu.user_id").
		Where("bu.business_id = ? AND bu.role = 'staff'", businessID).
		Order("bu.created_at DESC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	members := make([]StaffMember, len(rows))
	for i, r := range rows {
		members[i] = StaffMember{ID: r.ID, FirstName: r.FirstName, LastName: r.LastName,
			Email: r.Email, PhoneNumber: r.PhoneNumber, Role: r.Role, IsActive: r.IsActive, JoinedAt: r.JoinedAt}
	}
	return members, nil
}

func (s *AuthService) ResendVerification(emailAddr string) error {
	var user models.User
	if err := s.db.Where("email = ?", emailAddr).First(&user).Error; err != nil {
		return errors.New("user not found")
	}
	if user.EmailVerifiedAt != nil {
		return errors.New("email already verified")
	}
	token := uuid.NewString()
	expiresAt := time.Now().Add(48 * time.Hour)
	if err := s.db.Model(&user).Updates(map[string]interface{}{
		"verification_token": token, "verification_expires_at": expiresAt,
	}).Error; err != nil {
		return err
	}
	var bu models.BusinessUser
	var businessName string
	if s.db.Where("user_id = ?", user.ID).First(&bu).Error == nil {
		var business models.Business
		if s.db.First(&business, bu.BusinessID).Error == nil {
			businessName = business.Name
		}
	}
	email.SendVerificationEmail(user.Email, token, s.frontendURL, businessName)
	return nil
}
