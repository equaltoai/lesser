package handlers

import (
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestStatuses_CreateHelpers_BuildStatusValidationParams(t *testing.T) {
	t.Parallel()

	scheduledAt := time.Now().UTC().Format(time.RFC3339)
	req := &apimodels.CreateStatusRequest{
		Status:      "hello",
		Visibility:  VisibilityPublic,
		MediaIDs:    []string{"m1", "m2"},
		ScheduledAt: &scheduledAt,
		InReplyToID: "parent",
		SpoilerText: "spoiler",
		Language:    "en",
		Sensitive:   true,
		Poll:        &apimodels.Poll{Options: []string{"a", "b"}, ExpiresIn: 60, Multiple: true, HideTotals: true},
	}

	params := buildStatusValidationParams(req)

	require.Equal(t, "hello", params["status"])
	require.Equal(t, VisibilityPublic, params["visibility"])
	require.Equal(t, true, params["sensitive"])
	require.Equal(t, "spoiler", params["spoiler_text"])
	require.Equal(t, "en", params["language"])
	require.Equal(t, "parent", params["in_reply_to_id"])
	require.Equal(t, scheduledAt, params["scheduled_at"])

	mediaIDs, ok := params["media_ids"].([]any)
	require.True(t, ok)
	require.Len(t, mediaIDs, 2)

	pollMap, ok := params["poll"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 60, pollMap["expires_in"])
	require.Equal(t, true, pollMap["multiple"])
	require.Equal(t, true, pollMap["hide_totals"])
	options, ok := pollMap["options"].([]any)
	require.True(t, ok)
	require.Len(t, options, 2)
}

func TestStatuses_CreateHelpers_ParseScheduledAt(t *testing.T) {
	t.Parallel()

	_, err := parseScheduledAt(nil)
	require.Error(t, err)

	valid := time.Now().UTC().Format(time.RFC3339Nano)
	got, err := parseScheduledAt(&valid)
	require.NoError(t, err)
	require.False(t, got.IsZero())

	invalid := "not-a-timestamp"
	_, err = parseScheduledAt(&invalid)
	require.Error(t, err)
}

func TestStatuses_CreateHelpers_BuildScheduledPoll(t *testing.T) {
	t.Parallel()

	require.Nil(t, buildScheduledPoll(nil))

	poll := &apimodels.Poll{
		Options:    []string{"a", "b"},
		ExpiresIn:  120,
		Multiple:   true,
		HideTotals: false,
	}
	out := buildScheduledPoll(poll)
	require.NotNil(t, out)
	require.Equal(t, []string{"a", "b"}, out["options"])
	require.Equal(t, 120, out["expires_in"])
	require.Equal(t, true, out["multiple"])
	require.Equal(t, false, out["hide_totals"])
}

func TestStatuses_CreateHelpers_CreateNoteCommandFromStatusRequest(t *testing.T) {
	t.Parallel()

	claims := &auth.Claims{Username: "alice"}
	req := &apimodels.CreateStatusRequest{
		Status:     "hello",
		Visibility: VisibilityPublic,
		Poll: &apimodels.Poll{
			Options:    []string{"a", "b"},
			ExpiresIn:  120,
			Multiple:   true,
			HideTotals: true,
		},
	}

	cmd := createNoteCommandFromStatusRequest(claims, req, nil)
	require.NotNil(t, cmd)
	require.Equal(t, "alice", cmd.AuthorID)
	require.Equal(t, "hello", cmd.Content)
	require.Equal(t, VisibilityPublic, cmd.Visibility)
	require.Equal(t, []string{"a", "b"}, cmd.PollOptions)
	require.Equal(t, 120, cmd.PollExpiresIn)
	require.Equal(t, true, cmd.PollMultiple)
	require.Equal(t, true, cmd.PollHideTotals)
}
