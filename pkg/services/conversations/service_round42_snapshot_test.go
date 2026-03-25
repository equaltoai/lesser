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
		ConversationData: &models.ConversationSnapshot{Participants: []string{"arch", "scout"}},
	}))
	require.True(t, participantRecordSnapshotCorrupt(&models.ConversationParticipantRecord{
		ConversationData: &models.ConversationSnapshot{ID: "conv-1"},
	}))
	require.False(t, participantRecordSnapshotCorrupt(&models.ConversationParticipantRecord{
		ConversationData: &models.ConversationSnapshot{
			ID:           "conv-1",
			Participants: []string{"arch", "scout"},
		},
	}))
}
