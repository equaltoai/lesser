package repositories

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestUserRepository_extractObjectMetadata_ErrorBranches(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	_, err := repo.extractObjectMetadata(map[string]interface{}{"type": "Note"}, zap.NewNop())
	assert.Error(t, err)

	_, err = repo.extractObjectMetadata(map[string]interface{}{
		"id":           "note1",
		"type":         "Note",
		"attributedTo": "invalid_actor_id",
	}, zap.NewNop())
	assert.Error(t, err)
}

func TestUserRepository_HelperFunctions_CoverBranches(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	assert.Equal(t, []string{}, convertToStringSlice(nil))
	assert.Equal(t, []string{"a", "b"}, convertToStringSlice([]string{"a", "b"}))
	assert.Equal(t, []string{"a", "b"}, convertToStringSlice([]interface{}{"a", "b", 123}))
	assert.Equal(t, []string{"a"}, convertToStringSlice("a"))
	assert.Equal(t, []string{}, convertToStringSlice(123))

	assert.Equal(t, "value", getStringFromMap(map[string]interface{}{"k": "value"}, "k"))
	assert.Equal(t, "", getStringFromMap(map[string]interface{}{"k": 1}, "k"))

	assert.Equal(t, "fr", extractLanguage(map[string]interface{}{"language": "fr"}))
	assert.Equal(t, "de", extractLanguage(map[string]interface{}{"contentMap": map[string]interface{}{"de": "hallo"}}))
	assert.Equal(t, "en", extractLanguage(map[string]interface{}{}))

	assert.Equal(t, "direct", repo.determineVisibility(map[string]interface{}{
		"to": []string{"https://example.com/users/bob"},
	}))
	assert.Equal(t, "public", repo.determineVisibility(map[string]interface{}{
		"to": []string{activitypub.PublicAddress},
	}))
	assert.Equal(t, "unlisted", repo.determineVisibility(map[string]interface{}{
		"to": []string{"https://example.com/users/bob"},
		"cc": []string{activitypub.PublicAddress},
	}))
}
