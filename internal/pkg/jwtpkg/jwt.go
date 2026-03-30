// internal/pkg/jwtpkg/jwt.go
package jwtpkg

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	issuerApp   = "ogaos"
	issuerAdmin = "ogaos-admin"
)

// Claims is embedded in regular user (owner/staff) access tokens.
type Claims struct {
	UserID     uuid.UUID `json:"uid"`
	Email      string    `json:"email"`
	BusinessID uuid.UUID `json:"bid,omitempty"`
	Role       string    `json:"role"` // "owner" | "staff"
	IsPlatform bool      `json:"plt,omitempty"`
	jwt.RegisteredClaims
}

// AdminClaims is embedded in platform-admin access tokens.
// It uses a different JWT secret AND a different issuer, so a user token
// cannot be replayed on an admin endpoint even if the secrets were somehow shared.
type AdminClaims struct {
	UserID      uuid.UUID `json:"uid"`
	Email       string    `json:"email"`
	Role        string    `json:"role"` // "platform_super_admin" | "platform_support" etc.
	IsPlatform  bool      `json:"plt"`
	MFAVerified bool      `json:"mfa"`
	jwt.RegisteredClaims
}

// ── User tokens ───────────────────────────────────────────────────────────────

// GenerateAccessToken mints a short-lived JWT for regular users.
func GenerateAccessToken(
	userID uuid.UUID,
	email string,
	businessID uuid.UUID,
	role string,
	isPlatform bool,
	secret []byte,
	ttl time.Duration,
) (string, error) {
	claims := Claims{
		UserID:     userID,
		Email:      email,
		BusinessID: businessID,
		Role:       role,
		IsPlatform: isPlatform,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    issuerApp,
			Subject:   userID.String(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ParseAccessToken validates a user access token.
// It explicitly rejects any token whose issuer is issuerAdmin.
func ParseAccessToken(tokenStr string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return nil, errors.New("invalid or expired token")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("malformed token claims")
	}
	// Reject admin-issued tokens on user endpoints
	if claims.Issuer == issuerAdmin {
		return nil, errors.New("token issuer mismatch")
	}
	return claims, nil
}

// ── Admin tokens ──────────────────────────────────────────────────────────────

// GenerateAdminAccessToken mints a short-lived JWT for platform admins.
// It uses adminSecret (separate from user secret) and issuerAdmin.
func GenerateAdminAccessToken(
	userID uuid.UUID,
	email string,
	role string,
	mfaVerified bool,
	adminSecret []byte,
	ttl time.Duration,
) (string, error) {
	claims := AdminClaims{
		UserID:      userID,
		Email:       email,
		Role:        role,
		IsPlatform:  true,
		MFAVerified: mfaVerified,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    issuerAdmin,
			Subject:   userID.String(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(adminSecret)
}

// ParseAdminToken validates a platform admin access token.
// It explicitly rejects tokens whose issuer is issuerApp.
func ParseAdminToken(tokenStr string, adminSecret []byte) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AdminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return adminSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return nil, errors.New("invalid or expired token")
	}
	claims, ok := token.Claims.(*AdminClaims)
	if !ok {
		return nil, errors.New("malformed token claims")
	}
	if claims.Issuer != issuerAdmin {
		return nil, errors.New("token issuer mismatch")
	}
	if !claims.IsPlatform {
		return nil, errors.New("not a platform admin token")
	}
	return claims, nil
}
