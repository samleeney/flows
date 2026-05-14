package live

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"unicode/utf8"
)

// CanonicalFlowPath returns the absolute, symlink-resolved, cleaned path for a
// flow file. If the file does not exist or symlink resolution fails, it falls
// back to the cleaned absolute path.
func CanonicalFlowPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return filepath.Clean(abs), nil
	}
	return filepath.Clean(resolved), nil
}

// FlowKey returns the lowercase hex SHA-256 of a canonical flow path.
func FlowKey(canonicalPath string) string {
	sum := sha256.Sum256([]byte(canonicalPath))
	return hex.EncodeToString(sum[:])
}

// NewRunID returns a 32-char hex string from 16 crypto-random bytes.
func NewRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// NewToken returns a base64url-encoded 32-byte random token.
func NewToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// TruncatePreviewUTF8 returns up to maxBytes of s, cut on a UTF-8 rune
// boundary so the result is always valid UTF-8. It also reports the original
// byte length and whether truncation occurred.
func TruncatePreviewUTF8(s string, maxBytes int) (preview string, totalBytes int, truncated bool) {
	totalBytes = len(s)
	if totalBytes <= maxBytes {
		return s, totalBytes, false
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], totalBytes, true
}
