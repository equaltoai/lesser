package mastodon

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubEmojiRepo struct {
	emojis map[string]*storage.CustomEmoji
	errs   map[string]error
	calls  map[string]int
}

func (s *stubEmojiRepo) GetCustomEmoji(_ context.Context, shortcode string) (*storage.CustomEmoji, error) {
	if s.calls != nil {
		s.calls[shortcode]++
	}
	if err, ok := s.errs[shortcode]; ok && err != nil {
		return nil, err
	}
	if emoji, ok := s.emojis[shortcode]; ok {
		return emoji, nil
	}
	return nil, storage.ErrNotFound
}

func TestExtractShortcodes(t *testing.T) {
	shortcodes := ExtractShortcodes("hello :smile: :smile: :a_b:")
	assert.ElementsMatch(t, []string{"smile", "a_b"}, shortcodes)
	assert.Nil(t, ExtractShortcodes("no emojis here"))
}

func TestEmojiParser_ParseEmojis_NoMatches(t *testing.T) {
	parser := &EmojiParser{repo: &stubEmojiRepo{}}

	emojis, err := parser.ParseEmojis(context.Background(), "no emojis here")
	require.NoError(t, err)
	assert.Nil(t, emojis)
}

func TestEmojiParser_ParseEmojis_SkipsMissingAndDisabled(t *testing.T) {
	calls := map[string]int{}
	repo := &stubEmojiRepo{
		emojis: map[string]*storage.CustomEmoji{
			"smile":    {Shortcode: "smile", URL: "https://example.com/smile.png"},
			"disabled": {Shortcode: "disabled", URL: "https://example.com/disabled.png", Disabled: true},
		},
		calls: calls,
	}
	parser := &EmojiParser{repo: repo}

	emojis, err := parser.ParseEmojis(context.Background(), "hi :smile: :disabled: :missing: :smile:")
	require.NoError(t, err)
	require.Len(t, emojis, 1)
	assert.Equal(t, "smile", emojis[0].Shortcode)
	assert.NotNil(t, emojis[0].Emoji)

	// duplicates should only hit storage once
	assert.Equal(t, 1, calls["smile"])
}

func TestEmojiParser_ParseEmojis_PropagatesErrors(t *testing.T) {
	repo := &stubEmojiRepo{
		errs: map[string]error{
			"boom": errors.New("boom"),
		},
	}
	parser := &EmojiParser{repo: repo}

	_, err := parser.ParseEmojis(context.Background(), "hi :boom:")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get emoji boom")
}

func TestEmojiParser_ReplaceEmojisAndProcessContent(t *testing.T) {
	repo := &stubEmojiRepo{
		emojis: map[string]*storage.CustomEmoji{
			"smile": {Shortcode: "smile", URL: "https://example.com/smile.png"},
		},
	}
	parser := &EmojiParser{repo: repo}

	content := "hi :smile:"
	emojis, err := parser.ParseEmojis(context.Background(), content)
	require.NoError(t, err)

	replaced := parser.ReplaceEmojis(content, emojis)
	assert.NotContains(t, replaced, "hi :smile:")
	assert.Contains(t, replaced, `src="https://example.com/smile.png"`)

	processed, parsed, err := parser.ProcessContent(context.Background(), content)
	require.NoError(t, err)
	assert.Equal(t, replaced, processed)
	require.Len(t, parsed, 1)
}
