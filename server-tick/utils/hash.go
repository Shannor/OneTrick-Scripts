package utils

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// HashMap generates a hash for a given map.
// The map should have the same key-value pairs in any order to produce the same hash.
func HashMap(data any) (string, error) {
	// Serialize the map to JSON to ensure consistent ordering of keys
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	// Compute the SHA-256 hash of the serialized JSON
	hash := sha256.Sum256(jsonBytes)

	// Return the hash as a hexadecimal string
	return fmt.Sprintf("%x", hash), nil
}
