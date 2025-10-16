package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
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

// IDGenerator provides production-ready ID generation with multiple formats
type IDGenerator struct {
	entropy *ulid.MonotonicEntropy
	logger  *zap.Logger
}

// NewIDGenerator creates a new ID generator with cryptographically secure entropy
func NewIDGenerator(logger *zap.Logger) *IDGenerator {
	// Use cryptographically secure random source
	entropy := ulid.Monotonic(rand.Reader, 0)

	return &IDGenerator{
		entropy: entropy,
		logger:  logger,
	}
}

// GenerateULID generates a ULID (Universally Unique Lexicographically Sortable Identifier)
// ULIDs are preferred for database performance as they are:
// - Lexicographically sortable
// - 128-bit compatible with UUID
// - More compact string representation
// - Better for database indexing performance
func (g *IDGenerator) GenerateULID() string {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), g.entropy)
	return id.String()
}

// GenerateActivityPubID generates an ActivityPub-compatible ID with domain context
func (g *IDGenerator) GenerateActivityPubID(domain, objectType string) string {
	id := g.GenerateULID()

	// Ensure domain has proper protocol
	if !strings.HasPrefix(domain, "http") {
		domain = "https://" + domain
	}

	return fmt.Sprintf("%s/%s/%s", domain, objectType, id)
}

// GenerateStatusID generates a status ID with time-based prefix for better sorting
func (g *IDGenerator) GenerateStatusID() string {
	return "status_" + g.GenerateULID()
}

// GenerateActorID generates an actor ID with proper ActivityPub format
func (g *IDGenerator) GenerateActorID(domain, username string) string {
	// Ensure domain has proper protocol
	if !strings.HasPrefix(domain, "http") {
		domain = "https://" + domain
	}

	return fmt.Sprintf("%s/users/%s", domain, username)
}

// GenerateObjectID generates an object ID for ActivityPub objects
func (g *IDGenerator) GenerateObjectID(domain string) string {
	return g.GenerateActivityPubID(domain, "objects")
}

// GenerateActivityID generates an activity ID for ActivityPub activities
func (g *IDGenerator) GenerateActivityID(domain string) string {
	return g.GenerateActivityPubID(domain, "activities")
}

// GenerateNoteID generates a note ID with proper prefix
func (g *IDGenerator) GenerateNoteID() string {
	return "note_" + g.GenerateULID()
}

// GenerateMediaID generates a media ID with proper prefix
func (g *IDGenerator) GenerateMediaID() string {
	return "media_" + g.GenerateULID()
}

// GenerateSessionID generates a session ID for user sessions
func (g *IDGenerator) GenerateSessionID() string {
	return "session_" + g.GenerateULID()
}

// GenerateReportID generates a report ID for moderation reports
func (g *IDGenerator) GenerateReportID() string {
	return "report_" + g.GenerateULID()
}

// GenerateAuditLogID generates an audit log ID
func (g *IDGenerator) GenerateAuditLogID() string {
	return "audit_" + g.GenerateULID()
}

// GenerateOperationID generates an operation ID for bulk operations
func (g *IDGenerator) GenerateOperationID() string {
	return "op_" + g.GenerateULID()
}

// GenerateJobID generates a job ID for background jobs
func (g *IDGenerator) GenerateJobID() string {
	return "job_" + g.GenerateULID()
}

// GenerateExportID generates an export ID
func (g *IDGenerator) GenerateExportID() string {
	return "export_" + g.GenerateULID()
}

// GenerateImportID generates an import ID
func (g *IDGenerator) GenerateImportID() string {
	return "import_" + g.GenerateULID()
}

// GenerateConversationID generates a conversation ID
func (g *IDGenerator) GenerateConversationID() string {
	return "conv_" + g.GenerateULID()
}

// GenerateMessageID generates a message ID
func (g *IDGenerator) GenerateMessageID() string {
	return "msg_" + g.GenerateULID()
}

// GenerateRequestID generates a request ID for tracing
func (g *IDGenerator) GenerateRequestID() string {
	return "req_" + g.GenerateULID()
}

// GenerateStreamingSessionID generates a streaming session ID
func (g *IDGenerator) GenerateStreamingSessionID() string {
	return "stream_" + g.GenerateULID()
}

// GenerateVouchID generates a vouch ID for reputation system
func (g *IDGenerator) GenerateVouchID() string {
	return "vouch_" + g.GenerateULID()
}

// GenerateCommunityNoteID generates a community note ID
func (g *IDGenerator) GenerateCommunityNoteID() string {
	return "cn_" + g.GenerateULID()
}

// ShortID generates a shorter ID for cases where full ULID is too long
// Uses base32 encoding of 8 bytes for readability while maintaining uniqueness
func (g *IDGenerator) ShortID() string {
	id := g.GenerateULID()
	// Take first 13 characters of ULID for shorter representation
	return strings.ToLower(id[:13])
}

// ValidateULID validates if a string is a valid ULID
func (g *IDGenerator) ValidateULID(id string) bool {
	_, err := ulid.Parse(id)
	return err == nil
}

// ExtractTimestamp extracts the timestamp from a ULID
func (g *IDGenerator) ExtractTimestamp(id string) (time.Time, error) {
	parsed, err := ulid.Parse(id)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid ULID: %w", err)
	}

	return ulid.Time(parsed.Time()), nil
}

// IsOlderThan checks if a ULID-based ID is older than the given duration
func (g *IDGenerator) IsOlderThan(id string, duration time.Duration) bool {
	timestamp, err := g.ExtractTimestamp(id)
	if err != nil {
		g.logger.Warn("failed to extract timestamp from ID",
			zap.String("id", id),
			zap.Error(err))
		return false
	}

	return time.Since(timestamp) > duration
}

// Global instance for easy access across the codebase
var globalIDGenerator *IDGenerator

// InitGlobalIDGenerator initializes the global ID generator
func InitGlobalIDGenerator(logger *zap.Logger) {
	globalIDGenerator = NewIDGenerator(logger)
}

// Convenience functions that use the global generator

// GenerateID generates a ULID using the global generator
func GenerateID() string {
	if globalIDGenerator == nil {
		// Fallback to basic ULID generation if not initialized
		return ulid.Make().String()
	}
	return globalIDGenerator.GenerateULID()
}

// GenerateStatusID generates a status ID using the global generator
func GenerateStatusID() string {
	if globalIDGenerator == nil {
		return "status_" + ulid.Make().String()
	}
	return globalIDGenerator.GenerateStatusID()
}

// GenerateNoteIDULID generates a note ID using the global generator
func GenerateNoteIDULID() string {
	if globalIDGenerator == nil {
		return "note_" + ulid.Make().String()
	}
	return globalIDGenerator.GenerateNoteID()
}

// GenerateMediaIDULID generates a media ID using the global generator
func GenerateMediaIDULID() string {
	if globalIDGenerator == nil {
		return "media_" + ulid.Make().String()
	}
	return globalIDGenerator.GenerateMediaID()
}

// GenerateSessionIDULID generates a session ID using the global generator
func GenerateSessionIDULID() string {
	if globalIDGenerator == nil {
		return "session_" + ulid.Make().String()
	}
	return globalIDGenerator.GenerateSessionID()
}

// GenerateOperationIDULID generates an operation ID using the global generator
func GenerateOperationIDULID() string {
	if globalIDGenerator == nil {
		return "op_" + ulid.Make().String()
	}
	return globalIDGenerator.GenerateOperationID()
}

// GenerateJobIDULID generates a job ID using the global generator
func GenerateJobIDULID() string {
	if globalIDGenerator == nil {
		return "job_" + ulid.Make().String()
	}
	return globalIDGenerator.GenerateJobID()
}

// GenerateRequestIDULID generates a request ID using the global generator
func GenerateRequestIDULID() string {
	if globalIDGenerator == nil {
		return "req_" + ulid.Make().String()
	}
	return globalIDGenerator.GenerateRequestID()
}

// GenerateActivityPubIDULID generates an ActivityPub ID using the global generator
func GenerateActivityPubIDULID(domain, objectType string) string {
	if globalIDGenerator == nil {
		id := ulid.Make().String()
		if !strings.HasPrefix(domain, "http") {
			domain = "https://" + domain
		}
		return fmt.Sprintf("%s/%s/%s", domain, objectType, id)
	}
	return globalIDGenerator.GenerateActivityPubID(domain, objectType)
}
