// Package emoji provides utilities for parsing and handling custom emoji
// in ActivityPub messages and Mastodon-compatible APIs.
package emoji

import (
	"context"
	"regexp"
	"unicode"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// UnicodeEmojiRegex matches Unicode emoji characters
// This includes basic emojis, emoji with skin tone modifiers, and complex sequences
var UnicodeEmojiRegex = regexp.MustCompile(`[\x{1f300}-\x{1f5ff}]|[\x{1f600}-\x{1f64f}]|[\x{1f680}-\x{1f6ff}]|[\x{1f700}-\x{1f77f}]|[\x{1f780}-\x{1f7ff}]|[\x{1f800}-\x{1f8ff}]|[\x{1f900}-\x{1f9ff}]|[\x{1fa00}-\x{1fa6f}]|[\x{1fa70}-\x{1faff}]|[\x{2600}-\x{26ff}]|[\x{2700}-\x{27bf}]|[\x{1f1e6}-\x{1f1ff}]{2}`)

// Parser provides comprehensive emoji parsing functionality
type Parser struct {
	customEmojiParser *mastodon.EmojiParser
	logger            *zap.Logger
}

// NewParser creates a new emoji parser with both Unicode and custom emoji support
func NewParser(repos core.RepositoryStorage, logger *zap.Logger) *Parser {
	return &Parser{
		customEmojiParser: mastodon.NewEmojiParser(repos),
		logger:            logger,
	}
}

// ParsedResult contains both Unicode and custom emojis found in content
type ParsedResult struct {
	CustomEmojis  []mastodon.ParsedEmoji `json:"custom_emojis"`
	UnicodeEmojis []string               `json:"unicode_emojis"`
}

// ParseAll extracts both Unicode emojis and custom emoji shortcodes from content
func (p *Parser) ParseAll(ctx context.Context, content string) (*ParsedResult, error) {
	result := &ParsedResult{
		CustomEmojis:  []mastodon.ParsedEmoji{},
		UnicodeEmojis: []string{},
	}

	// Parse custom emojis (shortcodes like :custom_name:)
	customEmojis, err := p.customEmojiParser.ParseEmojis(ctx, content)
	if err != nil {
		p.logger.Warn("Failed to parse custom emojis", zap.Error(err))
		// Continue processing Unicode emojis even if custom emoji parsing fails
	} else {
		result.CustomEmojis = customEmojis
	}

	// Parse Unicode emojis
	unicodeEmojis := p.extractUnicodeEmojis(content)
	result.UnicodeEmojis = unicodeEmojis

	return result, nil
}

// extractUnicodeEmojis extracts unique Unicode emoji characters from content
func (p *Parser) extractUnicodeEmojis(content string) []string {
	matches := UnicodeEmojiRegex.FindAllString(content, -1)
	if err := common.ValidateSliceNotEmpty("matches", matches); err != nil {
		return []string{}
	}

	// Use map to avoid duplicates and filter valid emojis
	emojiMap := make(map[string]bool)
	for _, match := range matches {
		// Additional validation to ensure we have actual emoji characters
		if p.isValidUnicodeEmoji(match) {
			emojiMap[match] = true
		}
	}

	// Convert to slice
	emojis := make([]string, 0, len(emojiMap))
	for emoji := range emojiMap {
		emojis = append(emojis, emoji)
	}

	return emojis
}

// isValidUnicodeEmoji performs additional validation for Unicode emoji characters
func (p *Parser) isValidUnicodeEmoji(s string) bool {
	if err := common.ValidateRequiredParam("s", s); err != nil {
		return false
	}

	// Check if string contains emoji-like Unicode characters
	for _, r := range s {
		if p.isEmojiRune(r) {
			return true
		}
	}

	return false
}

// isEmojiRune checks if a rune is in an emoji Unicode block
func (p *Parser) isEmojiRune(r rune) bool {
	// Check primary emoji blocks first (most common)
	if p.isInPrimaryEmojiRange(r) {
		return true
	}

	// Check secondary emoji blocks
	if p.isInSecondaryEmojiRange(r) {
		return true
	}

	// Check individual emoji characters
	if p.isIndividualEmoji(r) {
		return true
	}

	// Check if it's a printable character that might be an emoji variant
	return unicode.IsGraphic(r) && !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// isInPrimaryEmojiRange checks if rune is in major emoji blocks
func (p *Parser) isInPrimaryEmojiRange(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1F5FF) || // Miscellaneous Symbols and Pictographs
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map Symbols
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols and Pictographs
		(r >= 0x1F1E6 && r <= 0x1F1FF) || // Regional Indicator Symbols
		(r >= 0x2600 && r <= 0x26FF) || // Miscellaneous Symbols
		(r >= 0x2700 && r <= 0x27BF) // Dingbats
}

// isInSecondaryEmojiRange checks if rune is in less common emoji blocks
//
//nolint:gocyclo // High complexity is acceptable for comprehensive emoji range checking
func (p *Parser) isInSecondaryEmojiRange(r rune) bool {
	return (r >= 0x1F700 && r <= 0x1F77F) || // Alchemical Symbols
		(r >= 0x1F780 && r <= 0x1F7FF) || // Geometric Shapes Extended
		(r >= 0x1F800 && r <= 0x1F8FF) || // Supplemental Arrows-C
		(r >= 0x1FA00 && r <= 0x1FA6F) || // Chess Symbols
		(r >= 0x1FA70 && r <= 0x1FAFF) || // Symbols and Pictographs Extended-A
		(r >= 0x2194 && r <= 0x2199) || // Arrows
		(r >= 0x21A9 && r <= 0x21AA) || // More arrows
		(r >= 0x2328 && r <= 0x232A) || // Keyboard symbols
		(r >= 0x23CF && r <= 0x23F3) || // Control symbols
		(r >= 0x23F8 && r <= 0x23FA) || // More control symbols
		(r >= 0x25AA && r <= 0x25AB) || // Geometric shapes
		(r >= 0x25FB && r <= 0x25FE) || // More geometric shapes
		(r >= 0x2614 && r <= 0x2615) || // Umbrella symbols
		(r >= 0x2618 && r <= 0x261D) || // Hand symbols
		(r >= 0x2626 && r <= 0x262A) || // Religious symbols
		(r >= 0x262E && r <= 0x262F) || // Peace symbols
		(r >= 0x2638 && r <= 0x263A) || // Wheel and face symbols
		(r >= 0x2648 && r <= 0x2653) || // Zodiac symbols
		(r >= 0x265F && r <= 0x2660) || // Chess pieces
		(r >= 0x2692 && r <= 0x2697) || // Tools
		(r >= 0x26A0 && r <= 0x26A1) || // Warning symbols
		(r >= 0x26A7 && r <= 0x26AA) || // Gender symbols
		(r >= 0x26AB && r <= 0x26B1) || // Circles and sports
		(r >= 0x26BD && r <= 0x26BE) || // Sport balls
		(r >= 0x26C4 && r <= 0x26C5) || // Snowman
		(r >= 0x26E9 && r <= 0x26EA) || // Church symbols
		(r >= 0x26F0 && r <= 0x26F5) || // Mountain and sports
		(r >= 0x26F7 && r <= 0x26FA) // More sports
}

// isIndividualEmoji checks specific individual emoji codepoints
func (p *Parser) isIndividualEmoji(r rune) bool {
	individualEmojis := []rune{
		0x203C, 0x2049, 0x2122, 0x2139, 0x231A, 0x231B, 0x24C2,
		0x25B6, 0x25C0, 0x260E, 0x2611, 0x2620, 0x2622, 0x2623,
		0x2640, 0x2642, 0x2663, 0x2665, 0x2666, 0x2668, 0x267B,
		0x267E, 0x267F, 0x2699, 0x269B, 0x269C, 0x26C8, 0x26CE,
		0x26CF, 0x26D1, 0x26D3, 0x26D4, 0x26FD,
	}

	for _, emoji := range individualEmojis {
		if r == emoji {
			return true
		}
	}
	return false
}

// GetForStatus extracts unique emojis from status content and returns them in Mastodon API format
func (p *Parser) GetForStatus(ctx context.Context, content string) ([]any, error) {
	parsed, err := p.ParseAll(ctx, content)
	if err != nil {
		return []any{}, err
	}

	// Convert custom emojis to Mastodon API format
	result := make([]any, 0, len(parsed.CustomEmojis))
	for _, customEmoji := range parsed.CustomEmojis {
		if customEmoji.Emoji != nil {
			result = append(result, map[string]any{
				"shortcode":         customEmoji.Emoji.Shortcode,
				"url":               customEmoji.Emoji.URL,
				"static_url":        customEmoji.Emoji.StaticURL,
				"visible_in_picker": customEmoji.Emoji.VisibleInPicker,
				"category":          customEmoji.Emoji.Category,
			})
		}
	}

	// Note: Unicode emojis are not included in the Mastodon API emoji list
	// They are handled client-side and don't need to be in the emojis array

	return result, nil
}

// ProcessContent processes content by parsing emojis and replacing custom emoji shortcodes with HTML
func (p *Parser) ProcessContent(ctx context.Context, content string) (string, []any, error) {
	// Process custom emojis and replace shortcodes with HTML
	processedContent, _, err := p.customEmojiParser.ProcessContent(ctx, content)
	if err != nil {
		p.logger.Warn("Failed to process custom emojis", zap.Error(err))
		processedContent = content // Use original content if processing fails
	}

	// Get emojis in API format
	apiEmojis, err := p.GetForStatus(ctx, content)
	if err != nil {
		p.logger.Warn("Failed to format emojis for API", zap.Error(err))
		return processedContent, []any{}, nil
	}

	return processedContent, apiEmojis, nil
}
