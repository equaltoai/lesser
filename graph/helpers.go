package graph

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/equaltoai/lesser/pkg/auth"
)

// generateID generates a unique ID for objects
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}

// floatPtr returns a pointer to a float64

// intPtr returns a pointer to an int

// boolPtr returns a pointer to a bool

// deriveVisibility determines the visibility level based on To and CC fields

// convertMentions extracts mentions from tags

// convertTags filters tags to exclude mentions

// convertAttachments converts attachment slice to pointer slice

// getTimeOrNow returns the time or current time if nil

// getUsernameFromContext extracts username from authentication context
func getUsernameFromContext(ctx context.Context) string {
	// Extract claims from context
	if claims, ok := ctx.Value(auth.ContextKeyClaims).(*auth.Claims); ok && claims != nil {
		return claims.Username
	}
	return ""
}

// GetUserID extracts user ID from authentication context
func GetUserID(ctx context.Context) string {
	// Try to get claims from context
	if claims, ok := ctx.Value(auth.ContextKeyClaims).(*auth.Claims); ok && claims != nil {
		return claims.Username // In this system, username is used as user ID
	}
	return ""
}

// convertToGraphQLObject converts storage objects to GraphQL model objects

// validateNoteInput validates the input for creating a note

// extractDomainFromActorID extracts the domain from an actor ID

// generateUniqueID generates a unique ID for objects

// determineAudience determines the To field based on visibility

// determineCCAudience determines the CC field based on visibility

// getSensitive safely extracts the sensitive flag

// getSpoilerText safely extracts the spoiler text

// buildTags builds tag array from hashtags and mentions

// buildAttachments builds attachment objects from media IDs

// shouldFederate determines if an activity should be federated based on visibility

// convertToGraphQLObject converts an ActivityPub object to GraphQL Object type

// getObjectActorID extracts the actor ID from an object

// determineModerationCategory categorizes the moderation reason

// Helper methods for getting object interaction counts

// calculateMissingPosts calculates the number of missing posts in a thread

// calculateAverageEngagement calculates average engagement for posts in the thread
