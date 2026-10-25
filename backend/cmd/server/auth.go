package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

const passwordIterations = 210000

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	derived := derivePasswordKey(password, salt, passwordIterations)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s",
		passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(derived)), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	var iterations int
	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil || iterations < 100000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual := derivePasswordKey(password, salt, iterations)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func derivePasswordKey(password string, salt []byte, iterations int) []byte {
	const keyLength = 32
	result := make([]byte, keyLength)
	for block := uint32(1); block == 1; block++ {
		mac := hmac.New(sha256.New, []byte(password))
		mac.Write(salt)
		var number [4]byte
		binary.BigEndian.PutUint32(number[:], block)
		mac.Write(number[:])
		u := mac.Sum(nil)
		copy(result, u)
		for round := 1; round < iterations; round++ {
			mac = hmac.New(sha256.New, []byte(password))
			mac.Write(u)
			u = mac.Sum(nil)
			for index := range result {
				result[index] ^= u[index]
			}
		}
	}
	return result
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
