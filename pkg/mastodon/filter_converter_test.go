package mastodon

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConverterImpl_ConvertFilterToMastodon(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	expiresAt := time.Now().UTC().Add(time.Hour)
	filter := &storage.Filter{
		ID:           "f1",
		Title:        "title",
		Context:      []string{"home"},
		ExpiresAt:    &expiresAt,
		FilterAction: "warn",
	}

	keywords := []*storage.FilterKeyword{
		{ID: "k1", Keyword: "foo", WholeWord: true},
	}
	statuses := []*storage.FilterStatus{
		{ID: "s1", StatusID: "st1"},
	}

	result := c.ConvertFilterToMastodon(filter, keywords, statuses)
	require.NotNil(t, result)
	assert.Equal(t, filter.ID, result.ID)
	assert.Equal(t, filter.Title, result.Title)
	assert.Equal(t, filter.Context, result.Context)
	assert.Equal(t, filter.FilterAction, result.FilterAction)
	require.NotNil(t, result.ExpiresAt)
	assert.Equal(t, expiresAt.Format(time.RFC3339), *result.ExpiresAt)
	require.Len(t, result.Keywords, 1)
	assert.Equal(t, keywords[0].Keyword, result.Keywords[0].Keyword)
	require.Len(t, result.Statuses, 1)
	assert.Equal(t, statuses[0].StatusID, result.Statuses[0].StatusID)
}

func TestConverterImpl_ConvertFilterKeywordToV1(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	filter := &storage.Filter{Context: []string{"home"}, FilterAction: "hide"}
	keyword := &storage.FilterKeyword{ID: "k1", Keyword: "foo", WholeWord: true}

	v1 := c.ConvertFilterKeywordToV1(keyword, filter)
	require.NotNil(t, v1)
	assert.Equal(t, keyword.ID, v1.ID)
	assert.Equal(t, keyword.Keyword, v1.Phrase)
	assert.True(t, v1.Irreversible)
	assert.True(t, v1.WholeWord)
}

func TestConverterImpl_ConvertMuteToRelationship(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	rel := &models.Relationship{}
	c.ConvertMuteToRelationship(rel, nil)
	assert.False(t, rel.Muting)
	assert.False(t, rel.MutingNotifications)

	c.ConvertMuteToRelationship(rel, &storage.Mute{HideNotifications: true})
	assert.True(t, rel.Muting)
	assert.True(t, rel.MutingNotifications)
}
