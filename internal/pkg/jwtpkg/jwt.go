// internal/pkg/jwt/jwt.go
package jwtpkg

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID     uuid.UUID `json:"user_id"`
	Email      string    `json:"email"`
	BusinessID uuid.UUID `json:"business_id,omitempty"`
	Role       string    `json:"role"` // "owner", "staff", "platform_super_admin", ...
	IsPlatform bool      `json:"is_platform,omitempty"`
	jwt.RegisteredClaims
}

func GenerateAccessToken(userID uuid.UUID, email string, businessID uuid.UUID, role string, isPlatform bool, secret []byte, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID:     userID,
		Email:      email,
		BusinessID: businessID,
		Role:       role,
		IsPlatform: isPlatform,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "ogaos",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func ParseAccessToken(tokenStr string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return nil, err
	}
	return token.Claims.(*Claims), nil
}
