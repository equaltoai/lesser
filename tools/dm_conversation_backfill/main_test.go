package main

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestParticipantsForStatus_UsesRecipientFieldsAndIgnoresContentMentions(t *testing.T) {
	status := &models.Status{
		AuthorUsername: "medic",
		AuthorID:       "https://sim.example.com/users/medic",
		Content:        "hello @arch and @pilot, looping back to @arch",
		ToRecipients:   []string{"https://sim.example.com/users/arch"},
		CcRecipients:   []string{"https://sim.example.com/users/scout"},
	}

	participants, ok := participantsForStatus(status)
	require.True(t, ok)
	require.Equal(t, []string{"arch", "medic", "scout"}, participants)
}

func TestParticipantsForStatus_PreservesRemoteRecipientActorIDs(t *testing.T) {
	status := &models.Status{
		AuthorUsername: "medic",
		AuthorID:       "https://sim.example.com/users/medic",
		ToRecipients: []string{
			"https://remote.example/users/arch",
		},
	}

	participants, ok := participantsForStatus(status)
	require.True(t, ok)
	require.Equal(t, []string{"https://remote.example/users/arch", "medic"}, participants)
}

func TestParticipantsForStatus_FallsBackToNoteAudience(t *testing.T) {
	status := &models.Status{
		AuthorUsername: "medic",
		AuthorID:       "https://sim.example.com/users/medic",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				To:  []string{"https://sim.example.com/users/arch"},
				BCC: []string{"https://sim.example.com/users/scout"},
			},
		},
	}

	participants, ok := participantsForStatus(status)
	require.True(t, ok)
	require.Equal(t, []string{"arch", "medic", "scout"}, participants)
}

func TestParticipantsForStatus_RejectsMissingRecipient(t *testing.T) {
	status := &models.Status{
		AuthorUsername: "medic",
		Content:        "hello there",
	}

	participants, ok := participantsForStatus(status)
	require.False(t, ok)
	require.Nil(t, participants)
}

func TestParticipantsForStatus_RejectsContentOnlyMentions(t *testing.T) {
	status := &models.Status{
		AuthorUsername: "medic",
		Content:        "hello @arch",
	}

	participants, ok := participantsForStatus(status)
	require.False(t, ok)
	require.Nil(t, participants)
}

func TestStatusNeedsConversationNormalization(t *testing.T) {
	status := &models.Status{
		StatusID:       "status-1",
		ConversationID: "old-conv",
		GSI3PK:         "CONVERSATION#old-conv",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				Type: "Note",
				ID:   "https://example.com/users/medic/statuses/status-1",
			},
			ConversationID: "old-conv",
		},
	}

	require.True(t, statusNeedsConversationNormalization(status, "new-conv"))
	status.ConversationID = "new-conv"
	status.GSI3PK = "CONVERSATION#new-conv"
	status.Note.ConversationID = "new-conv"
	require.False(t, statusNeedsConversationNormalization(status, "new-conv"))
}

func TestBuildConversationRecord_UsesThreadAndPreservesEarlierCreateTime(t *testing.T) {
	earliest := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	latest := earliest.Add(2 * time.Hour)

	thread := []*models.Status{
		{StatusID: "latest", AuthorUsername: "arch", PublishedAt: latest},
		{StatusID: "first", AuthorUsername: "medic", PublishedAt: earliest},
	}

	existing := &models.Conversation{
		ID:                "conv-1",
		CreatedAt:         earliest.Add(-1 * time.Hour),
		UpdatedAt:         latest.Add(30 * time.Minute),
		TotalMessageCount: 99,
	}

	record := buildConversationRecord("conv-1", []string{"arch", "medic"}, thread, existing)
	require.Equal(t, "conv-1", record.ID)
	require.Equal(t, []string{"arch", "medic"}, record.Participants)
	require.Equal(t, "latest", record.LastStatusID)
	require.Equal(t, latest, record.LastMessageTime)
	require.Equal(t, existing.CreatedAt, record.CreatedAt)
	require.Equal(t, existing.UpdatedAt, record.UpdatedAt)
	require.EqualValues(t, 99, record.TotalMessageCount)
}

func TestStatusPublishedAt_FallsBackDeterministically(t *testing.T) {
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	updated := created.Add(5 * time.Minute)

	require.Equal(t, time.Unix(0, 0).UTC(), statusPublishedAt(&models.Status{}))
	require.Equal(t, created, statusPublishedAt(&models.Status{CreatedAt: created}))
	require.Equal(t, updated, statusPublishedAt(&models.Status{UpdatedAt: updated}))
}
