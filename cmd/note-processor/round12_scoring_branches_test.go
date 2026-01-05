package main

import (
	"context"
	"errors"
	"testing"
	"time"

	comprehendTypes "github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/equaltoai/lesser/pkg/storage"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNoteProcessor_ConvertToComprehendLanguageCode_CoversCases(t *testing.T) {
	np := &NoteProcessor{logger: zap.NewNop()}

	cases := []struct {
		in   string
		want comprehendTypes.LanguageCode
	}{
		{"en", comprehendTypes.LanguageCodeEn},
		{"english", comprehendTypes.LanguageCodeEn},
		{"es", comprehendTypes.LanguageCodeEs},
		{"spanish", comprehendTypes.LanguageCodeEs},
		{"español", comprehendTypes.LanguageCodeEs},
		{"fr", comprehendTypes.LanguageCodeFr},
		{"français", comprehendTypes.LanguageCodeFr},
		{"de", comprehendTypes.LanguageCodeDe},
		{"deutsch", comprehendTypes.LanguageCodeDe},
		{"it", comprehendTypes.LanguageCodeIt},
		{"italiano", comprehendTypes.LanguageCodeIt},
		{"pt", comprehendTypes.LanguageCodePt},
		{"português", comprehendTypes.LanguageCodePt},
		{"ar", comprehendTypes.LanguageCodeAr},
		{"العربية", comprehendTypes.LanguageCodeAr},
		{"hi", comprehendTypes.LanguageCodeHi},
		{"हिन्दी", comprehendTypes.LanguageCodeHi},
		{"ja", comprehendTypes.LanguageCodeJa},
		{"日本語", comprehendTypes.LanguageCodeJa},
		{"ko", comprehendTypes.LanguageCodeKo},
		{"한국어", comprehendTypes.LanguageCodeKo},
		{"zh", comprehendTypes.LanguageCodeZh},
		{"中文", comprehendTypes.LanguageCodeZh},
		{"zh-tw", comprehendTypes.LanguageCodeZhTw},
		{"繁體中文", comprehendTypes.LanguageCodeZhTw},
		{"unknown", comprehendTypes.LanguageCodeEn},
	}

	for _, tc := range cases {
		require.Equal(t, tc.want, np.convertToComprehendLanguageCode(tc.in))
	}
}

func TestNoteProcessor_ReputationScoring_CoversThresholdsAndBuckets(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	userRepo := testingmocks.NewMockUserRepositoryInterface()
	userRepo.On("GetUser", mock.Anything, "trusted").Return(&storage.User{CreatedAt: now.AddDate(-2, 0, 0)}, nil)
	userRepo.On("GetUser", mock.Anything, "established").Return(&storage.User{CreatedAt: now.AddDate(0, 0, -120)}, nil)
	userRepo.On("GetUser", mock.Anything, "weekold").Return(&storage.User{CreatedAt: now.AddDate(0, 0, -10)}, nil)
	userRepo.On("GetUser", mock.Anything, "brandnew").Return(&storage.User{CreatedAt: now.AddDate(0, 0, -1)}, nil)

	relRepo := testingmocks.NewMockRelationshipRepository()
	relRepo.On("GetFollowerCount", mock.Anything, "followerErr").Return(int64(0), errors.New("boom"))
	relRepo.On("GetFollowerCount", mock.Anything, "followingErr").Return(int64(10), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "followingErr").Return(int64(0), errors.New("boom"))
	relRepo.On("GetFollowerCount", mock.Anything, "tooFew").Return(int64(9), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "tooFew").Return(int64(1), nil)
	relRepo.On("GetFollowerCount", mock.Anything, "zeroFollowing").Return(int64(10), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "zeroFollowing").Return(int64(0), nil)
	relRepo.On("GetFollowerCount", mock.Anything, "optimal").Return(int64(20), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "optimal").Return(int64(10), nil)
	relRepo.On("GetFollowerCount", mock.Anything, "one").Return(int64(10), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "one").Return(int64(10), nil)
	relRepo.On("GetFollowerCount", mock.Anything, "some").Return(int64(10), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "some").Return(int64(15), nil)
	relRepo.On("GetFollowerCount", mock.Anything, "minimal").Return(int64(10), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "minimal").Return(int64(40), nil)

	noteRepo := testingmocks.NewMockCommunityNoteRepository()

	noteRepo.On("GetUserVotingHistory", mock.Anything, "historyErr", mock.Anything).
		Return(([]*storage.CommunityNoteVote)(nil), errors.New("no history"))
	noteRepo.On("GetUserVotingHistory", mock.Anything, "tooShort", mock.Anything).
		Return([]*storage.CommunityNoteVote{{NoteID: "n1"}}, nil)

	// Accuracy 0.8 bucket (4/5), including one note retrieval error to cover that branch.
	votes08 := []*storage.CommunityNoteVote{
		{NoteID: "n1", Helpful: true, VoteType: "not_helpful", Weight: 1},
		{NoteID: "n2", Helpful: false, VoteType: "helpful", Weight: 1},
		{NoteID: "n3", Helpful: false, VoteType: "helpful", Weight: 1},
		{NoteID: "n4", Helpful: false, VoteType: "helpful", Weight: 1},
		{NoteID: "missing", Helpful: false, VoteType: "helpful", Weight: 1},
	}
	noteRepo.On("GetUserVotingHistory", mock.Anything, "bucket08", mock.Anything).Return(votes08, nil)
	noteRepo.On("GetCommunityNote", mock.Anything, "n1").Return(&storage.CommunityNote{ID: "n1", Status: "accepted", Score: 1.0}, nil)
	noteRepo.On("GetCommunityNote", mock.Anything, "n2").Return(&storage.CommunityNote{ID: "n2", Status: "rejected", Score: 0.6}, nil) // helpful by score
	noteRepo.On("GetCommunityNote", mock.Anything, "n3").Return(&storage.CommunityNote{ID: "n3", Status: "accepted", Score: 1.0}, nil)
	noteRepo.On("GetCommunityNote", mock.Anything, "n4").Return(&storage.CommunityNote{ID: "n4", Status: "accepted", Score: 1.0}, nil)
	noteRepo.On("GetCommunityNote", mock.Anything, "missing").Return((*storage.CommunityNote)(nil), errors.New("nope"))

	// Accuracy 0.5 bucket (3/5).
	votes05 := []*storage.CommunityNoteVote{
		{NoteID: "a1", VoteType: "helpful"},
		{NoteID: "a2", VoteType: "helpful"},
		{NoteID: "a3", VoteType: "helpful"},
		{NoteID: "a4", VoteType: "not_helpful"},
		{NoteID: "a5", VoteType: "not_helpful"},
	}
	noteRepo.On("GetUserVotingHistory", mock.Anything, "bucket05", mock.Anything).Return(votes05, nil)
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5"} {
		noteRepo.On("GetCommunityNote", mock.Anything, id).Return(&storage.CommunityNote{ID: id, Status: "accepted", Score: 1.0}, nil)
	}

	// Accuracy 0.2 bucket (2/5).
	votes02 := []*storage.CommunityNoteVote{
		{NoteID: "b1", VoteType: "helpful"},
		{NoteID: "b2", VoteType: "helpful"},
		{NoteID: "b3", VoteType: "not_helpful"},
		{NoteID: "b4", VoteType: "not_helpful"},
		{NoteID: "b5", VoteType: "not_helpful"},
	}
	noteRepo.On("GetUserVotingHistory", mock.Anything, "bucket02", mock.Anything).Return(votes02, nil)
	for _, id := range []string{"b1", "b2", "b3", "b4", "b5"} {
		noteRepo.On("GetCommunityNote", mock.Anything, id).Return(&storage.CommunityNote{ID: id, Status: "accepted", Score: 1.0}, nil)
	}

	// Accuracy 0.0 bucket (1/5).
	votes00 := []*storage.CommunityNoteVote{
		{NoteID: "c1", VoteType: "helpful"},
		{NoteID: "c2", VoteType: "not_helpful"},
		{NoteID: "c3", VoteType: "not_helpful"},
		{NoteID: "c4", VoteType: "not_helpful"},
		{NoteID: "c5", VoteType: "not_helpful"},
	}
	noteRepo.On("GetUserVotingHistory", mock.Anything, "bucket00", mock.Anything).Return(votes00, nil)
	for _, id := range []string{"c1", "c2", "c3", "c4", "c5"} {
		noteRepo.On("GetCommunityNote", mock.Anything, id).Return(&storage.CommunityNote{ID: id, Status: "accepted", Score: 1.0}, nil)
	}

	modRepo := testingmocks.NewMockModerationRepository()
	modRepo.On("GetModerationEventsByObject", mock.Anything, "moderationErr", mock.Anything, mock.Anything).
		Return(([]*storage.ModerationEvent)(nil), "", errors.New("boom"))

	old := now.AddDate(0, 0, -120)
	modRepo.On("GetModerationEventsByObject", mock.Anything, "severe", mock.Anything, mock.Anything).Return([]*storage.ModerationEvent{
		{EventType: "ban", CreatedAt: old}, // filtered out
		{EventType: "ban", CreatedAt: now},
		{EventType: "ban", CreatedAt: now},
		{EventType: "ban", CreatedAt: now},
		{EventType: "ban", CreatedAt: now},
	}, "", nil)
	modRepo.On("GetModerationEventsByObject", mock.Anything, "high", mock.Anything, mock.Anything).Return([]*storage.ModerationEvent{
		{EventType: "suspend", CreatedAt: now},
		{EventType: "ban", CreatedAt: now},
	}, "", nil)
	modRepo.On("GetModerationEventsByObject", mock.Anything, "moderate", mock.Anything, mock.Anything).Return([]*storage.ModerationEvent{
		{EventType: "warn", CreatedAt: now},
		{EventType: "warning", CreatedAt: now},
		{EventType: "warn", CreatedAt: now},
	}, "", nil)
	modRepo.On("GetModerationEventsByObject", mock.Anything, "minor", mock.Anything, mock.Anything).Return([]*storage.ModerationEvent{
		{EventType: "report", CreatedAt: now},
	}, "", nil)

	np := &NoteProcessor{
		logger:            zap.NewNop(),
		userRepo:          userRepo,
		relationshipRepo:  relRepo,
		communityNoteRepo: noteRepo,
		moderationRepo:    modRepo,
	}

	require.Equal(t, 1.0, np.calculateAccountAgeScore(ctx, "trusted"))
	require.Equal(t, 0.7, np.calculateAccountAgeScore(ctx, "established"))
	require.Equal(t, 0.3, np.calculateAccountAgeScore(ctx, "weekold"))
	require.Equal(t, 0.0, np.calculateAccountAgeScore(ctx, "brandnew"))
	require.Equal(t, 0.0, (&NoteProcessor{logger: zap.NewNop()}).calculateAccountAgeScore(ctx, "any"))

	require.Equal(t, 0.0, np.calculateSocialScore(ctx, "followerErr"))
	require.Equal(t, 0.0, np.calculateSocialScore(ctx, "followingErr"))
	require.Equal(t, 0.0, np.calculateSocialScore(ctx, "tooFew"))
	require.Equal(t, 1.0, np.calculateSocialScore(ctx, "zeroFollowing"))
	require.Equal(t, 0.8, np.calculateSocialScore(ctx, "optimal"))
	require.Equal(t, 0.5, np.calculateSocialScore(ctx, "one"))
	require.Equal(t, 0.3, np.calculateSocialScore(ctx, "some"))
	require.Equal(t, 0.1, np.calculateSocialScore(ctx, "minimal"))
	require.Equal(t, 0.0, (&NoteProcessor{logger: zap.NewNop()}).calculateSocialScore(ctx, "any"))

	require.Equal(t, 0.0, np.calculateVotingHistoryScore(ctx, "historyErr"))
	require.Equal(t, 0.0, np.calculateVotingHistoryScore(ctx, "tooShort"))
	require.Equal(t, 0.8, np.calculateVotingHistoryScore(ctx, "bucket08"))
	require.Equal(t, 0.5, np.calculateVotingHistoryScore(ctx, "bucket05"))
	require.Equal(t, 0.2, np.calculateVotingHistoryScore(ctx, "bucket02"))
	require.Equal(t, 0.0, np.calculateVotingHistoryScore(ctx, "bucket00"))

	require.Equal(t, 0.0, np.calculateModerationPenalty(ctx, "moderationErr"))
	require.Equal(t, 1.0, np.calculateModerationPenalty(ctx, "severe"))
	require.Equal(t, 0.8, np.calculateModerationPenalty(ctx, "high"))
	require.Equal(t, 0.5, np.calculateModerationPenalty(ctx, "moderate"))
	require.Equal(t, 0.2, np.calculateModerationPenalty(ctx, "minor"))
	require.Equal(t, 0.0, (&NoteProcessor{logger: zap.NewNop()}).calculateModerationPenalty(ctx, "any"))
}

