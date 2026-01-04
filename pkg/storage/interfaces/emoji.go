// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// EmojiRepository defines the interface for custom emoji operations.
// This handles custom emoji creation, retrieval, search, and usage tracking.
type EmojiRepository interface {
	// ===== Core Emoji Operations =====

	// CreateCustomEmoji creates a new custom emoji
	CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error

	// GetCustomEmoji retrieves a custom emoji by shortcode
	GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error)

	// GetCustomEmojis retrieves all custom emojis (not disabled)
	GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error)

	// UpdateCustomEmoji updates an existing custom emoji
	UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error

	// DeleteCustomEmoji deletes a custom emoji
	DeleteCustomEmoji(ctx context.Context, shortcode string) error

	// ===== Remote Emoji Operations =====

	// GetRemoteEmoji retrieves a remote emoji by shortcode and domain
	GetRemoteEmoji(ctx context.Context, shortcode, domain string) (*storage.CustomEmoji, error)

	// ===== Category and Search Operations =====

	// GetCustomEmojisByCategory retrieves custom emojis by category
	GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error)

	// SearchEmojis performs sophisticated emoji searches with relevance scoring
	SearchEmojis(ctx context.Context, query string, limit int) ([]*storage.CustomEmoji, error)

	// ===== Popularity and Usage Operations =====

	// GetPopularEmojis retrieves emojis by popularity score, optionally filtered by domain
	GetPopularEmojis(ctx context.Context, domain string, limit int) ([]*storage.CustomEmoji, error)

	// IncrementEmojiUsage increments the usage count for an emoji
	IncrementEmojiUsage(ctx context.Context, shortcode string) error
}
