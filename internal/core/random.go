package core

import (
	"crypto/rand"
	"encoding/hex"
)

// RandomHex returns n random bytes as hex (identifiers that must not collide).
func RandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
