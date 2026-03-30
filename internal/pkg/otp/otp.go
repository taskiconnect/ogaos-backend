// internal/pkg/otp/otp.go
package otp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
)

// GenerateOTP generates a secure 6-digit OTP
func GenerateOTP() (string, error) {
	max := big.NewInt(1000000) // 0-999999
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Ensure 6 digits by padding with zeros if necessary
	otp := fmt.Sprintf("%06d", n.Int64())
	return otp, nil
}

// HashOTP hashes the OTP for secure storage
func HashOTP(otp string) string {
	hash := sha256.Sum256([]byte(otp))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// VerifyOTP verifies if the provided OTP matches the stored hash
func VerifyOTP(otp, hash string) bool {
	return HashOTP(otp) == hash
}
