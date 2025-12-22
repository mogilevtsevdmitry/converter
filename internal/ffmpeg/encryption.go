package ffmpeg

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// EncryptionInfo holds HLS encryption information
type EncryptionInfo struct {
	Key        []byte // 16 bytes for AES-128
	IV         []byte // 16 bytes initialization vector
	KeyPath    string // Local path to key file
	KeyInfoPath string // Local path to key info file
	KeyURL     string // URL where key will be accessible
}

// GenerateEncryption generates encryption key and IV for HLS AES-128
func GenerateEncryption(hlsDir string, jobID uuid.UUID, keyURLTemplate string) (*EncryptionInfo, error) {
	// Generate random 16-byte key
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Generate random 16-byte IV
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Create paths
	keyPath := filepath.Join(hlsDir, "encryption.key")
	keyInfoPath := filepath.Join(hlsDir, "encryption.keyinfo")

	// Build key URL
	keyURL := buildKeyURL(keyURLTemplate, jobID)
	if keyURL == "" {
		// Default: relative path (key will be in same directory as playlist)
		keyURL = "encryption.key"
	}

	// Write key file (binary)
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("failed to write key file: %w", err)
	}

	// Write key info file for FFmpeg
	// Format:
	// Line 1: Key URI (what goes in the playlist)
	// Line 2: Path to the key file
	// Line 3: IV (optional, hex string)
	keyInfoContent := fmt.Sprintf("%s\n%s\n%s\n", keyURL, keyPath, hex.EncodeToString(iv))
	if err := os.WriteFile(keyInfoPath, []byte(keyInfoContent), 0600); err != nil {
		return nil, fmt.Errorf("failed to write key info file: %w", err)
	}

	return &EncryptionInfo{
		Key:         key,
		IV:          iv,
		KeyPath:     keyPath,
		KeyInfoPath: keyInfoPath,
		KeyURL:      keyURL,
	}, nil
}

// buildKeyURL replaces placeholders in the URL template
func buildKeyURL(template string, jobID uuid.UUID) string {
	if template == "" {
		return ""
	}

	result := template
	result = strings.ReplaceAll(result, "{job_id}", jobID.String())
	result = strings.ReplaceAll(result, "{jobId}", jobID.String())

	return result
}

// KeyHex returns the key as a hex string
func (e *EncryptionInfo) KeyHex() string {
	return hex.EncodeToString(e.Key)
}

// IVHex returns the IV as a hex string
func (e *EncryptionInfo) IVHex() string {
	return hex.EncodeToString(e.IV)
}
