package domain

import (
	"crypto/rand"
	"strings"
)

// NewID returns prefix + "_" + 12 random base32 characters, e.g. "ride_k7q2mz3abcde".
func NewID(prefix string) string {
	return prefix + "_" + strings.ToLower(rand.Text()[:12])
}
