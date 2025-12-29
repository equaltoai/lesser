package mastodon

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
)

// EmojiRegex matches emoji shortcodes in text
// Format: :shortcode: where shortcode contains alphanumeric characters and underscores
var EmojiRegex = regexp.MustCompile(`:([a-zA-Z0-9_]+):`)

// EmojiParser handles parsing and replacing emoji shortcodes in content
type EmojiParser struct {
	store core.RepositoryStorage
}

// NewEmojiParser creates a new emoji parser
func NewEmojiParser(store core.RepositoryStorage) *EmojiParser {
	return &EmojiParser{
		store: store,
	}
}

// ParsedEmoji represents a parsed emoji from content
type ParsedEmoji struct {
	Shortcode string
	Emoji     *storage.CustomEmoji
}

// ParseEmojis extracts emoji shortcodes from content and looks them up
func (p *EmojiParser) ParseEmojis(ctx context.Context, content string) ([]ParsedEmoji, error) {
	matches := EmojiRegex.FindAllStringSubmatch(content, -1)
	if err := common.ValidateSliceNotEmpty("matches", matches); err != nil {
		return nil, nil
	}

	// Use map to avoid duplicates
	emojiMap := make(map[string]*storage.CustomEmoji)

	for _, match := range matches {
		if len(match) > 1 {
			shortcode := match[1]

			// Skip if already processed
			if _, exists := emojiMap[shortcode]; exists {
				continue
			}

			// Look up emoji in storage
			emoji, err := p.store.Emoji().GetCustomEmoji(ctx, shortcode)
			if err != nil {
				// If emoji not found, skip it (leave as plain text)
				if errors.Is(err, storage.ErrNotFound) {
					continue
				}
				// For other errors, return the error
				return nil, fmt.Errorf("failed to get emoji %s: %w", shortcode, err)
			}

			// Only include visible emojis
			if !emoji.Disabled {
				emojiMap[shortcode] = emoji
			}
		}
	}

	// Convert to slice
	result := make([]ParsedEmoji, 0, len(emojiMap))
	for shortcode, emoji := range emojiMap {
		result = append(result, ParsedEmoji{
			Shortcode: shortcode,
			Emoji:     emoji,
		})
	}

	return result, nil
}

// ReplaceEmojis replaces emoji shortcodes in content with HTML img tags
func (p *EmojiParser) ReplaceEmojis(content string, emojis []ParsedEmoji) string {
	result := content

	for _, parsed := range emojis {
		if parsed.Emoji == nil {
			continue
		}

		// Create img tag
		imgTag := fmt.Sprintf(
			`<img class="emojione" alt=":%s:" title=":%s:" src="%s" />`,
			parsed.Shortcode,
			parsed.Shortcode,
			parsed.Emoji.URL,
		)

		// Replace all occurrences of :shortcode: with img tag
		pattern := fmt.Sprintf(`:%s:`, parsed.Shortcode)
		result = strings.ReplaceAll(result, pattern, imgTag)
	}

	return result
}

// ProcessContent parses emojis in content and returns both the processed content and used emojis
func (p *EmojiParser) ProcessContent(ctx context.Context, content string) (string, []ParsedEmoji, error) {
	// Parse emojis
	emojis, err := p.ParseEmojis(ctx, content)
	if err != nil {
		return content, nil, err
	}

	// Replace emojis in content if any found
	if len(emojis) > 0 {
		content = p.ReplaceEmojis(content, emojis)
	}

	return content, emojis, nil
}

// ExtractShortcodes extracts just the shortcodes from content without looking them up
// Useful for validation or when you just need the shortcode list
func ExtractShortcodes(content string) []string {
	matches := EmojiRegex.FindAllStringSubmatch(content, -1)
	if err := common.ValidateSliceNotEmpty("matches", matches); err != nil {
		return nil
	}

	// Use map to avoid duplicates
	shortcodeMap := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			shortcodeMap[match[1]] = true
		}
	}

	// Convert to slice
	shortcodes := make([]string, 0, len(shortcodeMap))
	for shortcode := range shortcodeMap {
		shortcodes = append(shortcodes, shortcode)
	}

	return shortcodes
}
