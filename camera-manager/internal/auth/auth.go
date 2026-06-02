package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

const (
	passwordScheme     = "pbkdf2_sha256"
	passwordIterations = 210000
	saltBytes          = 16
	hashBytes          = 32
	tokenBytes         = 32
)

func IsRole(value string) bool {
	return value == RoleAdmin || value == RoleViewer
}

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("password is required")
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := pbkdf2SHA256([]byte(password), salt, passwordIterations, hashBytes)
	return fmt.Sprintf("%s$%d$%s$%s",
		passwordScheme,
		passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(storedHash, password string) bool {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 4 || parts[0] != passwordScheme {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 10000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) == 0 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(got, expected) == 1
}

func NewSessionToken() (string, error) {
	token := make([]byte, tokenBytes)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hashLen := sha256.Size
	blocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	for block := 1; block <= blocks; block++ {
		u := pbkdf2Block(password, salt, iterations, block)
		out = append(out, u...)
	}
	return out[:keyLen]
}

func pbkdf2Block(password, salt []byte, iterations, block int) []byte {
	mac := hmac.New(sha256.New, password)
	_, _ = mac.Write(salt)
	_, _ = mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
	u := mac.Sum(nil)
	result := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, password)
		_, _ = mac.Write(u)
		u = mac.Sum(nil)
		for j := range result {
			result[j] ^= u[j]
		}
	}
	return result
}
