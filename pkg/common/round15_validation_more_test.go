package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationHelpers_BooleanSchemeAndMedia(t *testing.T) {
	t.Run("ValidateBooleanString", func(t *testing.T) {
		assert.True(t, ValidateBooleanString(StringTrue))
		assert.True(t, ValidateBooleanString("1"))
		assert.True(t, ValidateBooleanString("yes"))
		assert.False(t, ValidateBooleanString("on"))
		assert.False(t, ValidateBooleanString("no"))
	})

	t.Run("ValidateHTTPScheme", func(t *testing.T) {
		assert.NoError(t, ValidateHTTPScheme(SchemeHTTP))
		assert.NoError(t, ValidateHTTPScheme(SchemeHTTPS))

		err := ValidateHTTPScheme("ftp")
		require.Error(t, err)
		var ve ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, "scheme", ve.Field)
	})

	t.Run("ValidateMediaType and IsProcessableMediaType", func(t *testing.T) {
		assert.NoError(t, ValidateMediaType(""))
		assert.NoError(t, ValidateMediaType("Image"))
		assert.NoError(t, ValidateMediaType("Video"))
		assert.NoError(t, ValidateMediaType("Document"))
		assert.NoError(t, ValidateMediaType("Audio"))

		err := ValidateMediaType("Unknown")
		require.Error(t, err)
		var ve ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, "media_type", ve.Field)

		assert.True(t, IsProcessableMediaType("Image"))
		assert.True(t, IsProcessableMediaType("Video"))
		assert.True(t, IsProcessableMediaType("Document"))
		assert.False(t, IsProcessableMediaType("Audio"))
		assert.False(t, IsProcessableMediaType("Other"))
	})

	t.Run("ValidateStatusState", func(t *testing.T) {
		assert.NoError(t, ValidateStatusState(""))
		assert.NoError(t, ValidateStatusState("pending"))
		assert.NoError(t, ValidateStatusState("completed"))
		assert.NoError(t, ValidateStatusState("failed"))
		assert.NoError(t, ValidateStatusState("processing"))
		assert.NoError(t, ValidateStatusState("ready"))

		err := ValidateStatusState("nope")
		require.Error(t, err)
		var ve ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, "status", ve.Field)
	})
}

func TestValidationHelpers_Entities(t *testing.T) {
	t.Run("ValidateUserEntity", func(t *testing.T) {
		require.Error(t, ValidateUserEntity("", ""))

		err := ValidateUserEntity("alice", "not-an-email")
		require.Error(t, err)
		var ve ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, "email", ve.Field)

		assert.NoError(t, ValidateUserEntity("alice", ""))
		assert.NoError(t, ValidateUserEntity("alice", "a@b"))
	})

	t.Run("ValidateActorEntity", func(t *testing.T) {
		err := ValidateActorEntity("", "alice")
		require.Error(t, err)
		var ve ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, "actor_id", ve.Field)

		require.Error(t, ValidateActorEntity("actor-1", "bad!"))
		assert.NoError(t, ValidateActorEntity("actor-1", "alice"))
	})

	t.Run("ValidateStatusEntity", func(t *testing.T) {
		err := ValidateStatusEntity("", "hello", "public")
		require.Error(t, err)

		err = ValidateStatusEntity("status-1", "", "public")
		require.Error(t, err)

		err = ValidateStatusEntity("status-1", "hello", "nope")
		require.Error(t, err)
		var ve ValidationError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, "visibility", ve.Field)

		assert.NoError(t, ValidateStatusEntity("status-1", "hello", "")) // defaults to public
		assert.NoError(t, ValidateStatusEntity("status-1", "hello", "public"))
	})
}

func TestValidationHelpers_SearchQueriesAndTimelineLimits(t *testing.T) {
	t.Run("ParseStatusTimelineLimit and ParseAccountStatusesLimit return default on invalid input", func(t *testing.T) {
		limit, err := ParseStatusTimelineLimit("not-a-number")
		require.NoError(t, err)
		assert.Equal(t, 20, limit)

		limit, err = ParseAccountStatusesLimit("not-a-number")
		require.NoError(t, err)
		assert.Equal(t, 20, limit)
	})

	t.Run("ValidateRepositorySearchQuery and ValidateNormalizedQuery", func(t *testing.T) {
		require.Error(t, ValidateRepositorySearchQuery("", 1))
		require.Error(t, ValidateRepositorySearchQuery("a", 2))
		require.Error(t, ValidateRepositorySearchQuery(strings.Repeat("a", 501), 1))
		assert.NoError(t, ValidateRepositorySearchQuery("Hello", 1))

		normalized, err := ValidateNormalizedQuery("  HeLLo  ")
		require.NoError(t, err)
		assert.Equal(t, "hello", normalized)

		_, err = ValidateNormalizedQuery("")
		require.Error(t, err)
	})
}
