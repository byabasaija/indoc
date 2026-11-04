package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// ComputeFileSHA256 opens a file and streams its bytes through the SHA-256 algorithm.
// Streaming avoids loading entire files into memory, keeping the memory footprint O(1).
func ComputeFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to open file for hashing: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to compute hash: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ComputeStringSHA256 calculates the SHA-256 checksum of an in-memory string.
func ComputeStringSHA256(content string) string {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	return hex.EncodeToString(hasher.Sum(nil))
}

