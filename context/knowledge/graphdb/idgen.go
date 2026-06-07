package graphdb

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

// GenerateID creates a unique ID for lifecycle entities.
func GenerateID(prefix string) string {
	timestamp := time.Now().UnixNano()
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	return fmt.Sprintf("%s_%d_%s", prefix, timestamp, hex.EncodeToString(randomBytes))
}

// GenerateSequenceID creates a sequence-based ID for ordered entities (events, transitions).
func GenerateSequenceID(prefix string, sequence uint64) string {
	return fmt.Sprintf("%s_%010d", prefix, sequence)
}

// GenerateNumericID generates a random numeric ID.
func GenerateNumericID() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	return n.String()
}
