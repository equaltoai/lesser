// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/google/uuid"
)

// FeaturedTagRepository is a thread-safe in-memory implementation of interfaces.FeaturedTagRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type FeaturedTagRepository struct {
	mu sync.RWMutex

	// Featured tags storage: username -> []FeaturedTag
	tagsByUser map[string][]*storage.FeaturedTag

	// Tag usage tracking for suggestions: username -> tagName -> count
	tagUsage map[string]map[string]int
}

// NewFeaturedTagRepository creates a new in-memory featured tag repository
func NewFeaturedTagRepository() *FeaturedTagRepository {
	return &FeaturedTagRepository{
		tagsByUser: make(map[string][]*storage.FeaturedTag),
		tagUsage:   make(map[string]map[string]int),
	}
}

// ===== Core Featured Tag Operations =====

// CreateFeaturedTag creates a new featured tag for a user
func (r *FeaturedTagRepository) CreateFeaturedTag(_ context.Context, tag *storage.FeaturedTag) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tag == nil || tag.Username == "" || tag.Name == "" {
		return fmt.Errorf("tag username and name are required")
	}

	// Normalize tag name
	tagName := strings.TrimPrefix(strings.ToLower(tag.Name), "#")

	// Check if already featured
	for _, existingTag := range r.tagsByUser[tag.Username] {
		if strings.EqualFold(existingTag.Name, tagName) {
			return storage.ErrAlreadyExists
		}
	}

	// Create new featured tag
	newTag := &storage.FeaturedTag{
		ID:            uuid.New().String(),
		Username:      tag.Username,
		Name:          tagName,
		URL:           fmt.Sprintf("https://example.com/tags/%s", tagName),
		StatusesCount: tag.StatusesCount,
		LastStatusAt:  tag.LastStatusAt,
		CreatedAt:     time.Now(),
	}

	r.tagsByUser[tag.Username] = append(r.tagsByUser[tag.Username], newTag)

	// Update the original tag with generated values
	tag.ID = newTag.ID
	tag.Name = newTag.Name
	tag.URL = newTag.URL
	tag.CreatedAt = newTag.CreatedAt

	return nil
}

// DeleteFeaturedTag removes a featured tag
func (r *FeaturedTagRepository) DeleteFeaturedTag(_ context.Context, username, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tags := r.tagsByUser[username]
	if len(tags) == 0 {
		return storage.ErrNotFound
	}

	// Find and remove the tag
	normalizedName := strings.TrimPrefix(strings.ToLower(name), "#")
	for i, tag := range tags {
		if strings.EqualFold(tag.Name, normalizedName) {
			r.tagsByUser[username] = append(tags[:i], tags[i+1:]...)
			return nil
		}
	}

	return storage.ErrNotFound
}

// GetFeaturedTags returns all featured tags for a user
func (r *FeaturedTagRepository) GetFeaturedTags(_ context.Context, username string) ([]*storage.FeaturedTag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tags := r.tagsByUser[username]
	if tags == nil {
		return []*storage.FeaturedTag{}, nil
	}

	// Return a copy to prevent external modification
	result := make([]*storage.FeaturedTag, len(tags))
	copy(result, tags)
	return result, nil
}

// ===== Tag Suggestions =====

// GetTagSuggestions returns suggested tags based on user's usage
func (r *FeaturedTagRepository) GetTagSuggestions(_ context.Context, username string, limit int) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get already featured tags to exclude them
	featuredMap := make(map[string]bool)
	for _, tag := range r.tagsByUser[username] {
		featuredMap[strings.ToLower(tag.Name)] = true
	}

	// Get tag usage for this user
	usage := r.tagUsage[username]
	if usage == nil {
		return []string{}, nil
	}

	// Build suggestions excluding already featured tags
	type tagCount struct {
		tag   string
		count int
	}

	var suggestions []tagCount
	for tag, count := range usage {
		if !featuredMap[strings.ToLower(tag)] {
			suggestions = append(suggestions, tagCount{tag: tag, count: count})
		}
	}

	// Sort by count descending (simple bubble sort for small lists)
	for i := 0; i < len(suggestions); i++ {
		for j := i + 1; j < len(suggestions); j++ {
			if suggestions[j].count > suggestions[i].count {
				suggestions[i], suggestions[j] = suggestions[j], suggestions[i]
			}
		}
	}

	// Return top suggestions
	result := make([]string, 0, limit)
	for i := 0; i < len(suggestions) && i < limit; i++ {
		result = append(result, suggestions[i].tag)
	}

	return result, nil
}

// ===== Test Helper Methods =====

// RecordTagUsage records tag usage for suggestion generation (test helper)
func (r *FeaturedTagRepository) RecordTagUsage(username, tagName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.tagUsage[username] == nil {
		r.tagUsage[username] = make(map[string]int)
	}
	r.tagUsage[username][strings.ToLower(tagName)]++
}

// Clear clears all data (test helper)
func (r *FeaturedTagRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tagsByUser = make(map[string][]*storage.FeaturedTag)
	r.tagUsage = make(map[string]map[string]int)
}

// Ensure FeaturedTagRepository implements interfaces.FeaturedTagRepository
var _ interfaces.FeaturedTagRepository = (*FeaturedTagRepository)(nil)
