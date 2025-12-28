package repositories

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dmerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPollRepository_CreateAndGetAndHelpers(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &PollRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Poll](mockDB, "tbl", zap.NewNop(), nil, "PollRepository", "poll"),
		voteRepo:               NewEnhancedBaseRepository[*models.PollVote](mockDB, "tbl", zap.NewNop(), nil, "PollVoteRepository", "poll_vote"),
	}

	// CreatePoll validation
	err := repo.CreatePoll(context.Background(), &storage.Poll{StatusID: "s", Options: []string{"one"}})
	require.Error(t, err)

	// CreatePoll success (ID generated)
	mockQuery.On("Create").Return(nil).Once()
	p := &storage.Poll{StatusID: "s", CreatedBy: "alice", Options: []string{"one", "two"}}
	err = repo.CreatePoll(context.Background(), p)
	require.NoError(t, err)
	require.NotEmpty(t, p.ID)
	require.Len(t, p.VotesCount, 2)
	require.Equal(t, 0, p.VotersCount)

	// CreatePoll create error
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	err = repo.CreatePoll(context.Background(), &storage.Poll{ID: "p1", StatusID: "s", CreatedBy: "alice", Options: []string{"one", "two"}})
	require.Error(t, err)

	// GetPoll success: votes map expanded into per-option counts
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if m, ok := args.Get(0).(*models.Poll); ok {
			m.ID = "p1"
			m.StatusID = "s"
			m.CreatedBy = "alice"
			m.Options = []string{"one", "two"}
			m.Votes = map[string][]int{"u1": {0}, "u2": {1, 0}}
			m.ExpiresAt = time.Now().Add(time.Hour)
		}
	}).Return(nil).Once()
	got, err := repo.GetPoll(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, []int{2, 1}, got.VotesCount)

	// updatePollCounts uses BaseRepository.Update
	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	err = repo.updatePollCounts(context.Background(), got)
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestPollRepository_QueryAndVoteErrorBranches(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	_ = NewPollRepository(mockDB, "tbl", zap.NewNop(), nil)
	repo := &PollRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Poll](mockDB, "tbl", zap.NewNop(), nil, "PollRepository", "poll"),
		voteRepo:               NewEnhancedBaseRepository[*models.PollVote](mockDB, "tbl", zap.NewNop(), nil, "PollVoteRepository", "poll_vote"),
	}

	// GetPollByStatusID query error
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	_, err := repo.GetPollByStatusID(context.Background(), "s")
	require.Error(t, err)

	// GetPollByStatusID empty -> not found
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if slice, ok := args.Get(0).(*[]*models.Poll); ok {
			*slice = nil
		}
	}).Return(nil).Once()
	_, err = repo.GetPollByStatusID(context.Background(), "s")
	require.Error(t, err)

	// GetPollByStatusID success
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if slice, ok := args.Get(0).(*[]*models.Poll); ok {
			*slice = []*models.Poll{{
				ID:        "p1",
				StatusID:  "s",
				CreatedBy: "alice",
				Options:   []string{"one", "two"},
				Votes:     map[string][]int{"u1": {0}},
				ExpiresAt: time.Now().Add(time.Hour),
			}}
		}
	}).Return(nil).Once()
	_, err = repo.GetPollByStatusID(context.Background(), "s")
	require.NoError(t, err)

	// VoteOnPoll: expired poll
	expired := time.Now().Add(-time.Hour)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if m, ok := args.Get(0).(*models.Poll); ok {
			m.ID = "p1"
			m.StatusID = "s"
			m.CreatedBy = "alice"
			m.Options = []string{"one", "two"}
			m.ExpiresAt = expired
			m.Votes = map[string][]int{}
		}
	}).Return(nil).Once()
	err = repo.VoteOnPoll(context.Background(), "p1", "u1", []int{0})
	require.Error(t, err)

	// VoteOnPoll: invalid choice index
	future := time.Now().Add(time.Hour)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if m, ok := args.Get(0).(*models.Poll); ok {
			m.ID = "p1"
			m.StatusID = "s"
			m.CreatedBy = "alice"
			m.Options = []string{"one", "two"}
			m.ExpiresAt = future
			m.Votes = map[string][]int{}
		}
	}).Return(nil).Once()
	err = repo.VoteOnPoll(context.Background(), "p1", "u1", []int{2})
	require.Error(t, err)

	// VoteOnPoll: multiple choice constraint
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if m, ok := args.Get(0).(*models.Poll); ok {
			m.ID = "p1"
			m.StatusID = "s"
			m.CreatedBy = "alice"
			m.Options = []string{"one", "two"}
			m.Multiple = false
			m.ExpiresAt = future
			m.Votes = map[string][]int{}
		}
	}).Return(nil).Once()
	err = repo.VoteOnPoll(context.Background(), "p1", "u1", []int{0, 1})
	require.Error(t, err)

	// VoteOnPoll: already voted branch (voteRepo.Get returns nil)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if m, ok := args.Get(0).(*models.Poll); ok {
			m.ID = "p1"
			m.StatusID = "s"
			m.CreatedBy = "alice"
			m.Options = []string{"one", "two"}
			m.Multiple = true
			m.ExpiresAt = future
			m.Votes = map[string][]int{}
		}
	}).Return(nil).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	err = repo.VoteOnPoll(context.Background(), "p1", "u1", []int{0})
	require.Error(t, err)

	// VoteOnPoll: voteRepo.Get returns non-notfound error
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if m, ok := args.Get(0).(*models.Poll); ok {
			m.ID = "p1"
			m.StatusID = "s"
			m.CreatedBy = "alice"
			m.Options = []string{"one", "two"}
			m.Multiple = true
			m.ExpiresAt = future
			m.Votes = map[string][]int{}
		}
	}).Return(nil).Once()
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	err = repo.VoteOnPoll(context.Background(), "p1", "u2", []int{0})
	require.Error(t, err)

	// GetPollVotes and HasUserVoted success paths
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		s := reflect.MakeSlice(v.Elem().Type(), 0, 2)
		s = reflect.Append(s, reflect.ValueOf(&models.PollVote{VoterID: "u1", Choices: []int{0}}))
		s = reflect.Append(s, reflect.ValueOf(&models.PollVote{VoterID: "u2", Choices: []int{1}}))
		v.Elem().Set(s)
	}).Return(nil).Once()
	votes, err := repo.GetPollVotes(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, votes, 2)

	// GetPollVotes error path
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetPollVotes(context.Background(), "p1")
	require.Error(t, err)

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if v, ok := args.Get(0).(*models.PollVote); ok {
			v.VoterID = "u1"
			v.Choices = []int{1}
		}
	}).Return(nil).Once()
	has, choices, err := repo.HasUserVoted(context.Background(), "p1", "u1")
	require.NoError(t, err)
	require.True(t, has)
	require.Equal(t, []int{1}, choices)

	// HasUserVoted error branch
	mockQuery.On("First", mock.Anything).Return(dmerrors.NewError("GetItem", "PollVote", errors.New("boom"))).Once()
	_, _, err = repo.HasUserVoted(context.Background(), "p1", "u2")
	require.Error(t, err)

	// GetPoll error branch
	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()
	_, err = repo.GetPoll(context.Background(), "missing")
	require.Error(t, err)

	// updatePollCounts error branch
	mockQuery.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	err = repo.updatePollCounts(context.Background(), &storage.Poll{ID: "p1", StatusID: "s", CreatedBy: "alice", Options: []string{"one", "two"}})
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestPollRepository_VoteOnPoll_SuccessAndCreateVoteError(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &PollRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Poll](mockDB, "tbl", zap.NewNop(), nil, "PollRepository", "poll"),
		voteRepo:               NewEnhancedBaseRepository[*models.PollVote](mockDB, "tbl", zap.NewNop(), nil, "PollVoteRepository", "poll_vote"),
	}

	// 1) GetPoll (First) -> active poll
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if m, ok := args.Get(0).(*models.Poll); ok {
			m.ID = "p1"
			m.StatusID = "s"
			m.CreatedBy = "alice"
			m.Options = []string{"one", "two"}
			m.Multiple = true
			m.ExpiresAt = time.Now().Add(time.Hour)
			m.Votes = map[string][]int{}
		}
	}).Return(nil).Once()

	// 2) voteRepo.Get (First) -> not found
	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()

	// 3) voteRepo.Create (Create) -> success
	mockQuery.On("Create").Return(nil).Once()

	// 4) updatePollCounts (Update)
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	err := repo.VoteOnPoll(context.Background(), "p1", "u1", []int{0})
	require.NoError(t, err)

	// Now cover vote create error path
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if m, ok := args.Get(0).(*models.Poll); ok {
			m.ID = "p1"
			m.StatusID = "s"
			m.CreatedBy = "alice"
			m.Options = []string{"one", "two"}
			m.Multiple = true
			m.ExpiresAt = time.Now().Add(time.Hour)
			m.Votes = map[string][]int{}
		}
	}).Return(nil).Once()
	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(errors.New("boom")).Once()

	err = repo.VoteOnPoll(context.Background(), "p1", "u2", []int{1})
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}
