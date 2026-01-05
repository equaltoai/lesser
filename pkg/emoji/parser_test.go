package emoji

import (
	"testing"

	"github.com/stretchr/testify/require"
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
