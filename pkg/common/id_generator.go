package common

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
)

// GenerateNumericID generates a stable numeric ID from a username
// This ensures the same username always generates the same ID
func GenerateNumericID(username string) string {
	// Create a hash of the username
	hash := sha256.Sum256([]byte(username))

	// Take the first 8 bytes and convert to uint64
	id := binary.BigEndian.Uint64(hash[:8])

	// Ensure it's a positive number and within a reasonable range
	// Mask to 15 digits max to avoid client integer overflow issues
	id = id % 1000000000000000

	// Ensure it's not zero and has at least 10 digits
	if id < 1000000000 {
		id += 1000000000
	}

	return fmt.Sprintf("%d", id)
}

// GenerateNumericIDFromActorID generates a stable numeric ID from an ActivityPub actor ID
func GenerateNumericIDFromActorID(actorID string) string {
	// Extract username from actor ID
	// Handle patterns like:
	// - https://server.com/users/username
	// - https://server.com/@username

	username := actorID

	if strings.Contains(actorID, "/users/") {
		parts := strings.Split(actorID, "/users/")
		if len(parts) > 1 {
			username = parts[len(parts)-1]
		}
	} else if strings.Contains(actorID, "/@") {
		parts := strings.Split(actorID, "/@")
		if len(parts) > 1 {
			username = parts[len(parts)-1]
		}
	}

	// Remove any trailing slashes or query params
	if idx := strings.IndexAny(username, "/?#"); idx != -1 {
		username = username[:idx]
	}

	return GenerateNumericID(username)
}