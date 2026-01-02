package common

import (
	"context"
	stdErrors "errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeMastodonOp struct {
	validateErr error
	execErr     error
	response    interface{}

	metricsCalls int
}

func (o *fakeMastodonOp) Validate(context.Context) error { return o.validateErr }
func (o *fakeMastodonOp) Execute(context.Context) error  { return o.execErr }
func (o *fakeMastodonOp) GetResponse(context.Context) interface{} {
	return o.response
}
func (o *fakeMastodonOp) RecordMetrics(context.Context) error {
	o.metricsCalls++
	return nil
}

func TestMastodonBusinessLogic_MoreCoverage(t *testing.T) {
	t.Run("ValidateMastodonUsername cases", func(t *testing.T) {
		assert.Error(t, ValidateMastodonUsername(""))
		assert.Error(t, ValidateMastodonUsername("bad!"))
		assert.Error(t, ValidateMastodonUsername("a__b"))
		assert.Error(t, ValidateMastodonUsername("_alice"))
		assert.Error(t, ValidateMastodonUsername("alice_"))
		assert.Error(t, ValidateMastodonUsername(string(make([]byte, 31))))
		assert.NoError(t, ValidateMastodonUsername("alice_1"))
	})

	t.Run("ValidateMediaUpload enforces size and mime types", func(t *testing.T) {
		cfg := DefaultMastodonConfig()
		cfg.MediaUploadLimit = 10
		cfg.VideoUploadLimit = 10
		logic := NewMastodonBusinessLogic(cfg, zap.NewNop())

		assert.Error(t, logic.ValidateMediaUpload(make([]byte, 11), "image/png", MediaTypeImage))
		assert.Error(t, logic.ValidateMediaUpload(make([]byte, 11), "video/mp4", MediaTypeVideo))

		assert.Error(t, logic.ValidateMediaUpload(make([]byte, 1), "video/mp4", MediaTypeImage))
		assert.Error(t, logic.ValidateMediaUpload(make([]byte, 1), "image/png", MediaTypeVideo))
		assert.Error(t, logic.ValidateMediaUpload(make([]byte, 1), "image/png", MediaTypeAudio))

		assert.NoError(t, logic.ValidateMediaUpload(make([]byte, 1), "audio/mpeg", MediaTypeAudio))
	})

	t.Run("ValidateMastodonFilterKeyword", func(t *testing.T) {
		assert.Error(t, ValidateMastodonFilterKeyword(""))
		assert.Error(t, ValidateMastodonFilterKeyword(string(make([]byte, 41))))
		assert.NoError(t, ValidateMastodonFilterKeyword("hello"))
	})

	t.Run("ValidatePollOptions", func(t *testing.T) {
		cfg := DefaultMastodonConfig()
		cfg.MaxPollExpiry = 10 * time.Second
		logic := NewMastodonBusinessLogic(cfg, zap.NewNop())

		assert.Error(t, logic.ValidatePollOptions([]PollOption{{Title: "a"}}, 10))
		assert.Error(t, logic.ValidatePollOptions([]PollOption{{Title: ""}, {Title: "b"}}, 10))
		assert.Error(t, logic.ValidatePollOptions([]PollOption{{Title: string(make([]byte, 51))}, {Title: "b"}}, 10))
		assert.Error(t, logic.ValidatePollOptions([]PollOption{{Title: "a"}, {Title: "b"}}, -1))
		assert.Error(t, logic.ValidatePollOptions([]PollOption{{Title: "a"}, {Title: "b"}}, 1000))

		assert.NoError(t, logic.ValidatePollOptions([]PollOption{{Title: "a"}, {Title: "b"}}, 5))
	})

	t.Run("ValidateNotificationType", func(t *testing.T) {
		assert.NoError(t, ValidateNotificationType(string(NotificationMention)))
		assert.Error(t, ValidateNotificationType("nope"))
	})

	t.Run("ValidateMastodonOAuthScopes", func(t *testing.T) {
		scopes, err := ValidateMastodonOAuthScopes("")
		require.NoError(t, err)
		assert.Equal(t, []string{"read"}, scopes)

		scopes, err = ValidateMastodonOAuthScopes("read write  ")
		require.NoError(t, err)
		assert.Equal(t, []string{"read", "write"}, scopes)

		_, err = ValidateMastodonOAuthScopes("bad-scope")
		assert.Error(t, err)
	})

	t.Run("GenerateSnowflakeID is numeric", func(t *testing.T) {
		id := GenerateSnowflakeID()
		_, err := strconv.ParseUint(id, 10, 64)
		assert.NoError(t, err)
	})

	t.Run("ValidateMastodonID", func(t *testing.T) {
		assert.ErrorIs(t, ValidateMastodonID(""), ErrIDEmpty)
		assert.Error(t, ValidateMastodonID("not-a-number"))
		assert.NoError(t, ValidateMastodonID("123"))
	})

	t.Run("DefaultRateLimits", func(t *testing.T) {
		limits := DefaultRateLimits()
		assert.Greater(t, limits.PostsPerHour, 0)
	})

	t.Run("ExecuteMastodonAPIOperation validation and execution flows", func(t *testing.T) {
		logic := NewMastodonBusinessLogic(DefaultMastodonConfig(), zap.NewNop())

		_, err := logic.ExecuteMastodonAPIOperation(context.Background(), &fakeMastodonOp{
			validateErr: stdErrors.New("bad"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")

		op := &fakeMastodonOp{
			execErr:  stdErrors.New("boom"),
			response: map[string]any{"ok": true},
		}
		_, err = logic.ExecuteMastodonAPIOperation(context.Background(), op)
		require.Error(t, err)
		assert.GreaterOrEqual(t, op.metricsCalls, 1)

		op2 := &fakeMastodonOp{response: map[string]any{"ok": true}}
		resp, err := logic.ExecuteMastodonAPIOperation(context.Background(), op2)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"ok": true}, resp)
		assert.GreaterOrEqual(t, op2.metricsCalls, 1)
	})

	t.Run("timestamp helpers", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Millisecond)
		formatted := FormatMastodonTimestamp(now)
		parsed, err := ParseMastodonTimestamp(formatted)
		require.NoError(t, err)
		assert.Equal(t, now, parsed.UTC())

		_, err = ParseMastodonTimestamp("not-a-timestamp")
		assert.Error(t, err)
	})

	t.Run("SanitizeHTML and extract helpers", func(t *testing.T) {
		assert.Equal(t, "&lt;b&gt;x&amp;y&lt;/b&gt;", SanitizeHTML("<b>x&y</b>"))

		hashtags := ExtractHashtags("hello #one and #two_2")
		assert.Equal(t, []string{"one", "two_2"}, hashtags)

		mentions := ExtractMentions("hi @alice and @bob_2")
		assert.Equal(t, []string{"alice", "bob_2"}, mentions)
	})

	t.Run("MastodonAPIError helpers", func(t *testing.T) {
		e := NewMastodonAPIError("t", "d", 418)
		assert.Equal(t, 418, e.StatusCode)
		assert.Contains(t, e.Error(), "mastodon api error")
	})
}
