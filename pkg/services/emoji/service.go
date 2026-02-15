// Package emoji provides custom emoji management services for the Lesser ActivityPub server.
//
// This service handles all operations related to custom emojis including:
// - Creating, updating, and deleting custom emojis
// - Retrieving emojis for display in the UI
// - Managing emoji categories and visibility
// - Handling federation of custom emojis from remote instances
package emoji

import (
	"context"
	stdErrors "errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Service provides business logic for custom emoji operations
type Service struct {
	emojiRepo emojiRepository
	publisher streaming.Publisher
	logger    *zap.Logger
	domain    string
}

type emojiRepository interface {
	GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error)
	GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error)
	CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error
	UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error
	DeleteCustomEmoji(ctx context.Context, shortcode string) error
	GetRemoteEmoji(ctx context.Context, shortcode, domain string) (*storage.CustomEmoji, error)
	SearchEmojis(ctx context.Context, query string, limit int) ([]*storage.CustomEmoji, error)
	GetPopularEmojis(ctx context.Context, domain string, limit int) ([]*storage.CustomEmoji, error)
	IncrementEmojiUsage(ctx context.Context, shortcode string) error
}

var _ emojiRepository = (*repositories.EmojiRepository)(nil)

// NewService creates a new emoji service
func NewService(
	emojiRepo emojiRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
	domain string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{
		emojiRepo: emojiRepo,
		publisher: publisher,
		logger:    logger,
		domain:    domain,
	}
}

// Query and Command types for CQRS pattern

// GetEmojiQuery contains parameters for retrieving a single emoji
type GetEmojiQuery struct {
	Shortcode string `json:"shortcode" validate:"required"`
}

// ListEmojisQuery contains parameters for listing emojis
type ListEmojisQuery struct {
	OnlyLocal       bool   `json:"only_local"`       // Only show local emojis
	OnlyVisible     bool   `json:"only_visible"`     // Only show visible emojis
	Category        string `json:"category"`         // Filter by category
	IncludeDisabled bool   `json:"include_disabled"` // Include disabled emojis
}

// CreateEmojiCommand contains data needed to create a new emoji
type CreateEmojiCommand struct {
	Shortcode       string `json:"shortcode" validate:"required,min=2,max=30"`
	ImageURL        string `json:"image_url" validate:"required,url"`
	Category        string `json:"category"`
	VisibleInPicker bool   `json:"visible_in_picker"`
}

// UpdateEmojiCommand contains data needed to update an emoji
type UpdateEmojiCommand struct {
	Shortcode       string  `json:"shortcode" validate:"required"`
	Category        *string `json:"category"`
	VisibleInPicker *bool   `json:"visible_in_picker"`
	Disabled        *bool   `json:"disabled"`
}

// DeleteEmojiCommand contains data needed to delete an emoji
type DeleteEmojiCommand struct {
	Shortcode string `json:"shortcode" validate:"required"`
}

// CopyEmojiCommand contains data needed to copy a remote emoji
type CopyEmojiCommand struct {
	Shortcode    string `json:"shortcode" validate:"required"`
	Domain       string `json:"domain" validate:"required"`
	NewShortcode string `json:"new_shortcode"` // Optional, use original if not provided
}

// SearchEmojisQuery contains parameters for searching emojis
type SearchEmojisQuery struct {
	Query string `json:"query" validate:"required,min=1"`
	Limit int    `json:"limit"`
}

// GetPopularEmojisQuery contains parameters for getting popular emojis
type GetPopularEmojisQuery struct {
	Domain string `json:"domain"` // Optional, empty for local emojis
	Limit  int    `json:"limit"`
}

// IncrementUsageCommand contains data needed to increment emoji usage
type IncrementUsageCommand struct {
	Shortcode string `json:"shortcode" validate:"required"`
}

// Result types

// Result contains a single emoji
type Result struct {
	Emoji  *storage.CustomEmoji `json:"emoji"`
	Events []*streaming.Event   `json:"events"`
}

// ListResult contains multiple emojis
type ListResult struct {
	Emojis []*storage.CustomEmoji `json:"emojis"`
	Total  int                    `json:"total"`
	Events []*streaming.Event     `json:"events"`
}

// GetEmoji retrieves a single custom emoji by shortcode
func (s *Service) GetEmoji(ctx context.Context, query *GetEmojiQuery) (*storage.CustomEmoji, error) {
	s.logger.Info("getting emoji",
		zap.String("shortcode", query.Shortcode))

	emoji, err := s.emojiRepo.GetCustomEmoji(ctx, query.Shortcode)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return nil, errors.ErrEmojiNotFound
		}
		return nil, errors.ErrGetEmoji
	}

	return emoji, nil
}

// ListEmojis retrieves all custom emojis with optional filters
func (s *Service) ListEmojis(ctx context.Context, query *ListEmojisQuery) (*ListResult, error) {
	s.logger.Info("listing emojis",
		zap.Bool("only_local", query.OnlyLocal),
		zap.Bool("only_visible", query.OnlyVisible),
		zap.String("category", query.Category))

	// Get all emojis from repository
	emojis, err := s.emojiRepo.GetCustomEmojis(ctx)
	if err != nil {
		return nil, errors.ErrListEmojis
	}

	// Auto-seed default emojis if none exist
	if len(emojis) == 0 {
		s.seedDefaultEmojis(ctx)
		emojis, _ = s.emojiRepo.GetCustomEmojis(ctx)
	}

	// Apply filters
	filtered := s.filterEmojis(emojis, query)

	return &ListResult{
		Emojis: filtered,
		Total:  len(filtered),
		Events: nil,
	}, nil
}

// CreateEmoji creates a new custom emoji
func (s *Service) CreateEmoji(ctx context.Context, cmd *CreateEmojiCommand) (*Result, error) {
	s.logger.Info("creating emoji",
		zap.String("shortcode", cmd.Shortcode),
		zap.String("image_url", cmd.ImageURL))

	// Validate shortcode format
	if err := s.validateShortcode(cmd.Shortcode); err != nil {
		return nil, err
	}

	// Check if emoji already exists
	existing, _ := s.emojiRepo.GetCustomEmoji(ctx, cmd.Shortcode)
	if existing != nil {
		return nil, errors.ErrEmojiAlreadyExists
	}

	// Create the emoji
	emoji := &storage.CustomEmoji{
		Shortcode:       cmd.Shortcode,
		URL:             cmd.ImageURL,
		StaticURL:       cmd.ImageURL, // In production, this would be a processed static version
		VisibleInPicker: cmd.VisibleInPicker,
		Category:        cmd.Category,
		Domain:          "", // Local emoji
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := s.emojiRepo.CreateCustomEmoji(ctx, emoji); err != nil {
		return nil, errors.ErrCreateEmoji
	}

	// Emit events for real-time updates
	events := s.emitEmojiCreatedEvents(ctx, emoji)

	return &Result{
		Emoji:  emoji,
		Events: events,
	}, nil
}

// UpdateEmoji updates an existing custom emoji
func (s *Service) UpdateEmoji(ctx context.Context, cmd *UpdateEmojiCommand) (*Result, error) {
	s.logger.Info("updating emoji",
		zap.String("shortcode", cmd.Shortcode))

	// Get existing emoji
	emoji, err := s.emojiRepo.GetCustomEmoji(ctx, cmd.Shortcode)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return nil, errors.ErrEmojiNotFound
		}
		return nil, errors.ErrGetEmoji
	}

	// Check if it's a local emoji (can only update local emojis)
	if emoji.Domain != "" {
		return nil, errors.ErrCannotUpdateRemoteEmoji
	}

	// Apply updates
	updated := false
	if cmd.Category != nil {
		emoji.Category = *cmd.Category
		updated = true
	}
	if cmd.VisibleInPicker != nil {
		emoji.VisibleInPicker = *cmd.VisibleInPicker
		updated = true
	}
	if cmd.Disabled != nil {
		emoji.Disabled = *cmd.Disabled
		updated = true
	}

	if !updated {
		return &Result{Emoji: emoji, Events: nil}, nil
	}

	emoji.UpdatedAt = time.Now()

	// Update in repository
	if err := s.emojiRepo.UpdateCustomEmoji(ctx, emoji); err != nil {
		return nil, errors.ErrUpdateEmoji
	}

	// Emit events for real-time updates
	events := s.emitEmojiUpdatedEvents(ctx, emoji)

	return &Result{
		Emoji:  emoji,
		Events: events,
	}, nil
}

// DeleteEmoji deletes a custom emoji
func (s *Service) DeleteEmoji(ctx context.Context, cmd *DeleteEmojiCommand) error {
	s.logger.Info("deleting emoji",
		zap.String("shortcode", cmd.Shortcode))

	// Get existing emoji to verify it exists
	emoji, err := s.emojiRepo.GetCustomEmoji(ctx, cmd.Shortcode)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return errors.ErrEmojiNotFound
		}
		return errors.ErrGetEmoji
	}

	// Check if it's a local emoji (can only delete local emojis)
	if emoji.Domain != "" {
		return errors.ErrCannotDeleteRemoteEmoji
	}

	// Delete from repository
	if err := s.emojiRepo.DeleteCustomEmoji(ctx, cmd.Shortcode); err != nil {
		return errors.ErrDeleteEmoji
	}

	// Emit events for real-time updates
	s.emitEmojiDeletedEvents(ctx, emoji)

	return nil
}

// CopyRemoteEmoji copies a remote emoji to the local instance
func (s *Service) CopyRemoteEmoji(ctx context.Context, cmd *CopyEmojiCommand) (*Result, error) {
	s.logger.Info("copying remote emoji",
		zap.String("shortcode", cmd.Shortcode),
		zap.String("domain", cmd.Domain))

	// Get the remote emoji
	remoteEmoji, err := s.emojiRepo.GetRemoteEmoji(ctx, cmd.Shortcode, cmd.Domain)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return nil, errors.ErrRemoteEmojiNotFound
		}
		return nil, errors.ErrGetRemoteEmoji
	}

	// Determine new shortcode
	newShortcode := cmd.NewShortcode
	if err := common.ValidateRequiredParam("newShortcode", newShortcode); err != nil {
		newShortcode = cmd.Shortcode
	}

	// Validate new shortcode
	if err := s.validateShortcode(newShortcode); err != nil {
		return nil, err
	}

	// Check if local emoji with new shortcode already exists
	existing, _ := s.emojiRepo.GetCustomEmoji(ctx, newShortcode)
	if existing != nil {
		return nil, errors.ErrEmojiAlreadyExists
	}

	// Create local copy
	localEmoji := &storage.CustomEmoji{
		Shortcode:       newShortcode,
		URL:             remoteEmoji.URL,
		StaticURL:       remoteEmoji.StaticURL,
		VisibleInPicker: true,
		Category:        remoteEmoji.Category,
		Domain:          "", // Now a local emoji
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := s.emojiRepo.CreateCustomEmoji(ctx, localEmoji); err != nil {
		return nil, errors.ErrCreateLocalEmojiCopy
	}

	// Emit events
	events := s.emitEmojiCreatedEvents(ctx, localEmoji)

	return &Result{
		Emoji:  localEmoji,
		Events: events,
	}, nil
}

// SearchEmojis performs sophisticated emoji searches with relevance scoring
func (s *Service) SearchEmojis(ctx context.Context, query *SearchEmojisQuery) (*ListResult, error) {
	s.logger.Info("searching emojis",
		zap.String("query", query.Query),
		zap.Int("limit", query.Limit))

	// Set default limit if not provided
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100 // Cap at 100 for performance
	}

	// Perform the search using the repository's sophisticated search
	emojis, err := s.emojiRepo.SearchEmojis(ctx, query.Query, limit)
	if err != nil {
		return nil, errors.ErrSearchEmojis
	}

	return &ListResult{
		Emojis: emojis,
		Total:  len(emojis),
		Events: nil,
	}, nil
}

// GetPopularEmojis retrieves emojis by popularity score
func (s *Service) GetPopularEmojis(ctx context.Context, query *GetPopularEmojisQuery) (*ListResult, error) {
	s.logger.Info("getting popular emojis",
		zap.String("domain", query.Domain),
		zap.Int("limit", query.Limit))

	// Set default limit if not provided
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100 // Cap at 100 for performance
	}

	// Get popular emojis from repository
	emojis, err := s.emojiRepo.GetPopularEmojis(ctx, query.Domain, limit)
	if err != nil {
		return nil, errors.ErrGetPopularEmojis
	}

	return &ListResult{
		Emojis: emojis,
		Total:  len(emojis),
		Events: nil,
	}, nil
}

// IncrementUsage increments the usage count for an emoji
func (s *Service) IncrementUsage(ctx context.Context, cmd *IncrementUsageCommand) error {
	s.logger.Debug("incrementing emoji usage",
		zap.String("shortcode", cmd.Shortcode))

	// Increment usage in repository
	err := s.emojiRepo.IncrementEmojiUsage(ctx, cmd.Shortcode)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			// Don't treat missing emoji as error - might be from remote instance
			s.logger.Debug("emoji not found for usage increment",
				zap.String("shortcode", cmd.Shortcode))
			return nil
		}
		return errors.ErrIncrementEmojiUsage
	}

	return nil
}

// Helper methods

// filterEmojis applies query filters to emoji list
func (s *Service) filterEmojis(emojis []*storage.CustomEmoji, query *ListEmojisQuery) []*storage.CustomEmoji {
	filtered := make([]*storage.CustomEmoji, 0, len(emojis))

	for _, emoji := range emojis {
		// Filter by domain (local vs remote)
		if query.OnlyLocal && emoji.Domain != "" {
			continue
		}

		// Filter by visibility
		if query.OnlyVisible && !emoji.VisibleInPicker {
			continue
		}

		// Filter by disabled status
		if !query.IncludeDisabled && emoji.Disabled {
			continue
		}

		// Filter by category
		if query.Category != "" && emoji.Category != query.Category {
			continue
		}

		filtered = append(filtered, emoji)
	}

	return filtered
}

// validateShortcode validates emoji shortcode format
func (s *Service) validateShortcode(shortcode string) error {
	if err := common.ValidateIntRange("shortcode_length", len(shortcode), 2, 30); err != nil {
		return errors.ErrInvalidShortcode
	}

	// Check for valid characters (alphanumeric and underscore)
	if err := common.ValidateAlphanumericID("shortcode", shortcode); err != nil {
		return err
	}

	// Check for reserved words
	reserved := []string{"all", "none", "default", "custom"}
	lower := strings.ToLower(shortcode)
	for _, word := range reserved {
		if lower == word {
			return errors.ErrReservedShortcode
		}
	}

	return nil
}

// Event emission methods

func (s *Service) emitEmojiCreatedEvents(_ context.Context, emoji *storage.CustomEmoji) []*streaming.Event {
	if s.publisher == nil {
		return nil
	}

	event := &streaming.Event{
		Type:      "emoji.created",
		Stream:    "public",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"shortcode": emoji.Shortcode,
			"url":       emoji.URL,
			"category":  emoji.Category,
		},
	}

	// Emoji events are broadcast to all users since custom emojis are instance-wide.
	// Future enhancement: Consider targeted streaming to specific user groups or admin streams
	// for more granular emoji update notifications.
	s.logger.Info("emoji created event",
		zap.String("shortcode", emoji.Shortcode),
		zap.String("type", event.Type))

	return []*streaming.Event{event}
}

func (s *Service) emitEmojiUpdatedEvents(_ context.Context, emoji *storage.CustomEmoji) []*streaming.Event {
	if s.publisher == nil {
		return nil
	}

	event := &streaming.Event{
		Type:      "emoji.updated",
		Stream:    "public",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"shortcode":         emoji.Shortcode,
			"visible_in_picker": emoji.VisibleInPicker,
			"category":          emoji.Category,
			"disabled":          emoji.Disabled,
		},
	}

	// Log the event
	s.logger.Info("emoji updated event",
		zap.String("shortcode", emoji.Shortcode),
		zap.String("type", event.Type))

	return []*streaming.Event{event}
}

func (s *Service) emitEmojiDeletedEvents(_ context.Context, emoji *storage.CustomEmoji) {
	if s.publisher == nil {
		return
	}

	event := &streaming.Event{
		Type:      "emoji.deleted",
		Stream:    "public",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"shortcode": emoji.Shortcode,
		},
	}

	// Log the event
	s.logger.Info("emoji deleted event",
		zap.String("shortcode", emoji.Shortcode),
		zap.String("type", event.Type))
}

// seedDefaultEmojis creates a default set of custom emojis using Twemoji assets
func (s *Service) seedDefaultEmojis(ctx context.Context) {
	const twemojiBase = "https://cdn.jsdelivr.net/gh/twitter/twemoji@latest/assets/72x72/"

	defaults := []struct {
		shortcode string
		codepoint string
		category  string
	}{
		{"thumbsup", "1f44d", "Smileys & People"},
		{"thumbsdown", "1f44e", "Smileys & People"},
		{"heart", "2764", "Smileys & People"},
		{"fire", "1f525", "Smileys & People"},
		{"laughing", "1f606", "Smileys & People"},
		{"thinking", "1f914", "Smileys & People"},
		{"clap", "1f44f", "Smileys & People"},
		{"wave", "1f44b", "Smileys & People"},
		{"rocket", "1f680", "Travel & Places"},
		{"eyes", "1f440", "Smileys & People"},
		{"tada", "1f389", "Objects"},
		{"100", "1f4af", "Symbols"},
		{"star", "2b50", "Travel & Places"},
		{"sparkles", "2728", "Travel & Places"},
		{"rainbow", "1f308", "Travel & Places"},
		{"sun", "2600", "Travel & Places"},
		{"check_mark", "2705", "Symbols"},
		{"x", "274c", "Symbols"},
		{"warning", "26a0", "Symbols"},
		{"coffee", "2615", "Food & Drink"},
	}

	now := time.Now()
	seeded := 0
	for _, d := range defaults {
		url := twemojiBase + d.codepoint + ".png"
		emoji := &storage.CustomEmoji{
			Shortcode:       d.shortcode,
			URL:             url,
			StaticURL:       url,
			VisibleInPicker: true,
			Category:        d.category,
			Domain:          "",
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := s.emojiRepo.CreateCustomEmoji(ctx, emoji); err != nil {
			s.logger.Debug("failed to seed default emoji",
				zap.String("shortcode", d.shortcode),
				zap.Error(err))
			continue
		}
		seeded++
	}

	s.logger.Info("seeded default emojis", zap.Int("count", seeded))
}
