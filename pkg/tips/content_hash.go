package tips

import (
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

// ContentHashFromObjectID returns the canonical TipSplitter content hash for a Lesser object ID.
//
// Convention: keccak256(utf8(object.id)) as a 0x-prefixed 32-byte hex string.
func ContentHashFromObjectID(objectID string) string {
	id := strings.TrimSpace(objectID)
	if id == "" {
		return ""
	}
	return crypto.Keccak256Hash([]byte(id)).Hex()
}
