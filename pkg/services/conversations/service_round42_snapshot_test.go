package conversations

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound42_participantRecordSnapshotCorrupt(t *testing.T) {
	require.True(t, participantRecordSnapshotCorrupt(nil))
	require.True(t, participantRecordSnapshotCorrupt(&models.ConversationParticipantRecord{}))
	require.True(t, participantRecordSnapshotCorrupt(&models.ConversationParticipantRecord{
		Conversation: &models.Conversation{Participants: []string{"arch", "scout"}},
	}))
	require.False(t, participantRecordSnapshotCorrupt(&models.ConversationParticipantRecord{
		Conversation: &models.Conversation{ID: "conv-1"},
	}))
	require.False(t, participantRecordSnapshotCorrupt(&models.ConversationParticipantRecord{
		ConversationID: "conv-1",
	}))
	require.False(t, participantRecordSnapshotCorrupt(&models.ConversationParticipantRecord{
		Conversation: &models.Conversation{
			ID: "conv-1",
		},
	}))
}
