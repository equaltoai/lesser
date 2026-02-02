package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNoteProcessor_ProcessRecord_RecalculatesOnVoteInsert(t *testing.T) {
	noteRepo := testingmocks.NewMockCommunityNoteRepository()
	noteRepo.On("GetCommunityNote", mock.Anything, "n1").Return(&storage.CommunityNote{ID: "n1", Score: 0.2}, nil)
	noteRepo.On("GetCommunityNoteVotes", mock.Anything, "n1").Return([]*storage.CommunityNoteVote{
		{VoteType: "helpful", Helpful: true, Weight: 1},
		{VoteType: "not_helpful", Helpful: false, Weight: 1},
	}, nil)
	noteRepo.On("UpdateCommunityNoteScore", mock.Anything, "n1", mock.Anything, mock.Anything).Return(nil)

	np := &NoteProcessor{
		logger:            zap.NewNop(),
		communityNoteRepo: noteRepo,
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		EventID:   "evt-1",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("NOTE#n1"),
				"SK": events.NewStringAttribute("VOTE#v1"),
			},
		},
	}

	require.NoError(t, np.processRecord(context.Background(), "req", record))
	noteRepo.AssertExpectations(t)
}
