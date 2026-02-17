package webutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateHash creates a SHA-256 hash of the input string and returns it
// as a hexadecimal string.
func GenerateHash(data string) (string, error) {
	hasher := sha256.New()
	_, err := hasher.Write([]byte(data))
	if err != nil {
		return "", fmt.Errorf("failed to write data to hasher: %w", err)
	}
	// Sum returns the hash as a byte slice. Pass nil to allocate a new slice.
	hashBytes := hasher.Sum(nil)
	// Encode the byte slice into a hex string (64 characters for SHA-256).
	hashString := hex.EncodeToString(hashBytes)
	return hashString, nil
}
