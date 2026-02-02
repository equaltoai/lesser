package moderation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPatternManager_matchHash_TextImageAndMiss(t *testing.T) {
	pm := &PatternManager{}
	content := &ContentToModerate{
		TextHash:  "text-hash",
		ImageHash: "image-hash",
	}

	matched, got := pm.matchHash("image-hash", content)
	assert.True(t, matched)
	assert.Equal(t, "image-hash", got)

	matched, got = pm.matchHash("nope", content)
	assert.False(t, matched)
	assert.Empty(t, got)
}
