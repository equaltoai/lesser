package emoji

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestParser_extractUnicodeEmojis_Deduplicates(t *testing.T) {
	p := &Parser{}
	emojis := p.extractUnicodeEmojis("hello 😀😀 world 🌍")
	require.ElementsMatch(t, []string{"😀", "🌍"}, emojis)
}

func TestParser_extractUnicodeEmojis_Empty(t *testing.T) {
	p := &Parser{}
	emojis := p.extractUnicodeEmojis("no emojis here")
	require.Empty(t, emojis)
}

func TestParser_isValidUnicodeEmoji(t *testing.T) {
	p := &Parser{}
	require.True(t, p.isValidUnicodeEmoji("😀"))
	require.False(t, p.isValidUnicodeEmoji(""))
	require.False(t, p.isValidUnicodeEmoji("abc"))
}

func TestParser_ParseAll_And_APIHelpers_NoCustomEmojiShortcodes(t *testing.T) {
	p := NewParser(nil, zap.NewNop())

	out, err := p.ParseAll(context.Background(), "hello 😀 world")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, out.CustomEmojis, 0)
	require.ElementsMatch(t, []string{"😀"}, out.UnicodeEmojis)

	apiEmojis, err := p.GetForStatus(context.Background(), "hello 😀 world")
	require.NoError(t, err)
	require.Empty(t, apiEmojis)

	content, apiEmojis, err := p.ProcessContent(context.Background(), "hello 😀 world")
	require.NoError(t, err)
	require.Equal(t, "hello 😀 world", content)
	require.Empty(t, apiEmojis)
}

func TestParser_isEmojiRune_FallbackGraphicSymbols(t *testing.T) {
	p := &Parser{}
	require.True(t, p.isEmojiRune('*'))
}

func TestParser_isEmojiRune_SecondaryEmojiRange(t *testing.T) {
	p := &Parser{}
	require.True(t, p.isEmojiRune(rune(0x1F701)))
}

func TestParser_isIndividualEmoji_CoversMiss(t *testing.T) {
	p := &Parser{}
	require.True(t, p.isIndividualEmoji(0x203C))
	require.False(t, p.isIndividualEmoji('x'))
}
