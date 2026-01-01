// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// EmojiRepository is a thread-safe in-memory implementation of interfaces.EmojiRepository.
type EmojiRepository struct {
	mu sync.RWMutex
	// emojis stores emojis keyed by shortcode
	emojis map[string]*storage.CustomEmoji
	// remoteEmojis stores remote emojis keyed by "shortcode:domain"
	remoteEmojis map[string]*storage.CustomEmoji
}

// NewEmojiRepository creates a new in-memory emoji repository
func NewEmojiRepository() *EmojiRepository {
	return &EmojiRepository{
		emojis:       make(map[string]*storage.CustomEmoji),
		remoteEmojis: make(map[string]*storage.CustomEmoji),
	}
}

// CreateCustomEmoji creates a new custom emoji
func (r *EmojiRepository) CreateCustomEmoji(_ context.Context, emoji *storage.CustomEmoji) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.emojis[emoji.Shortcode]; exists {
		return storage.ErrAlreadyExists
	}
	r.emojis[emoji.Shortcode] = emoji
	return nil
}

// GetCustomEmoji retrieves a custom emoji by shortcode
func (r *EmojiRepository) GetCustomEmoji(_ context.Context, shortcode string) (*storage.CustomEmoji, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	emoji, exists := r.emojis[shortcode]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return emoji, nil
}


// GetCustomEmojis retrieves all custom emojis (not disabled)
func (r *EmojiRepository) GetCustomEmojis(_ context.Context) ([]*storage.CustomEmoji, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.CustomEmoji
	for _, emoji := range r.emojis {
		if !emoji.Disabled {
			result = append(result, emoji)
		}
	}
	return result, nil
}

// UpdateCustomEmoji updates an existing custom emoji
func (r *EmojiRepository) UpdateCustomEmoji(_ context.Context, emoji *storage.CustomEmoji) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.emojis[emoji.Shortcode]; !exists {
		return storage.ErrNotFound
	}
	r.emojis[emoji.Shortcode] = emoji
	return nil
}

// DeleteCustomEmoji deletes a custom emoji
func (r *EmojiRepository) DeleteCustomEmoji(_ context.Context, shortcode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.emojis, shortcode)
	return nil
}

// GetRemoteEmoji retrieves a remote emoji by shortcode and domain
func (r *EmojiRepository) GetRemoteEmoji(_ context.Context, shortcode, domain string) (*storage.CustomEmoji, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := shortcode + ":" + domain
	emoji, exists := r.remoteEmojis[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return emoji, nil
}

// GetCustomEmojisByCategory retrieves custom emojis by category
func (r *EmojiRepository) GetCustomEmojisByCategory(_ context.Context, category string) ([]*storage.CustomEmoji, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.CustomEmoji
	for _, emoji := range r.emojis {
		if emoji.Category == category && !emoji.Disabled {
			result = append(result, emoji)
		}
	}
	return result, nil
}

// SearchEmojis performs sophisticated emoji searches with relevance scoring
func (r *EmojiRepository) SearchEmojis(_ context.Context, query string, limit int) ([]*storage.CustomEmoji, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryLower := strings.ToLower(query)
	var result []*storage.CustomEmoji
	for _, emoji := range r.emojis {
		if emoji.Disabled {
			continue
		}
		if strings.Contains(strings.ToLower(emoji.Shortcode), queryLower) {
			result = append(result, emoji)
		}
	}

	// Sort by usage count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].UsageCount > result[j].UsageCount
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// GetPopularEmojis retrieves emojis by popularity score
func (r *EmojiRepository) GetPopularEmojis(_ context.Context, domain string, limit int) ([]*storage.CustomEmoji, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.CustomEmoji
	for _, emoji := range r.emojis {
		if emoji.Disabled {
			continue
		}
		if domain == "" || emoji.Domain == domain {
			result = append(result, emoji)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].UsageCount > result[j].UsageCount
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// IncrementEmojiUsage increments the usage count for an emoji
func (r *EmojiRepository) IncrementEmojiUsage(_ context.Context, shortcode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	emoji, exists := r.emojis[shortcode]
	if !exists {
		return storage.ErrNotFound
	}
	emoji.UsageCount++
	return nil
}

// Clear clears all data (test helper)
func (r *EmojiRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emojis = make(map[string]*storage.CustomEmoji)
	r.remoteEmojis = make(map[string]*storage.CustomEmoji)
}

// Ensure EmojiRepository implements interfaces.EmojiRepository
var _ interfaces.EmojiRepository = (*EmojiRepository)(nil)
