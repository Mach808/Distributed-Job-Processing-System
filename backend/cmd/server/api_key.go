package main

import (
	"crypto/sha256"
	"crypto/rand"
	"encoding/hex"
	"encoding/base64"
	"fmt"
)

// hashAPIKey returns the database representation of an API key.
// API keys should be generated with sufficient randomness; only this hash is
// stored in PostgreSQL.
func hashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate API key: %w", err)
	}
	return "ck_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}
