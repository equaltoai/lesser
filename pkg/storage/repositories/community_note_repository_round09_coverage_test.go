package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestCommunityNoteRepo(mockDB *mocks.MockDB) *CommunityNoteRepository {
	repo := NewCommunityNoteRepository(mockDB, "tbl", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)
	return repo
}

func TestCommunityNoteRepository_round09_votes_and_notes(t *testing.T) {
	ctx := context.Background()

	t.Run("get_user_voting_history_and_create_note", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestCommunityNoteRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "VOTES#u1").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.CommunityNoteVote)
			*dest = []models.CommunityNoteVote{
				{NoteID: "n1", VoterID: "u1", VoteType: "helpful", Helpful: true, Weight: 1.0, CreatedAt: time.Now()},
				{NoteID: "n2", VoterID: "u1", VoteType: "not_helpful", Helpful: false, Weight: 0.5, CreatedAt: time.Now()},
			}
		}).Return(nil).Once()

		hist, err := repo.GetUserVotingHistory(ctx, "u1", 2)
		require.NoError(t, err)
		require.Len(t, hist, 2)

		mockQuery.On("Create").Return(nil).Once()
		note := &storage.CommunityNote{
			ObjectID:         "obj",
			ObjectType:       "status",
			AuthorID:         "u1",
			Content:          "fact check",
			Language:         "en",
			VisibilityStatus: "visible",
		}
		require.NoError(t, repo.CreateCommunityNote(ctx, note))
		assert.NotEmpty(t, note.ID)
		assert.False(t, note.CreatedAt.IsZero())
	})

	t.Run("get_visible_notes_and_get_note", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestCommunityNoteRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.CommunityNote)
			*dest = models.CommunityNote{ID: "n1", ObjectID: "obj", ObjectType: "status", AuthorID: "u1", Content: "c", VisibilityStatus: "visible"}
		}).Return(nil).Once()

		n, err := repo.GetCommunityNote(ctx, "n1")
		require.NoError(t, err)
		assert.Equal(t, "n1", n.ID)

		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "OBJECT#obj#NOTES").Return(mockQuery).Once()
		mockQuery.On("Limit", 50).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.CommunityNote)
			*dest = []models.CommunityNote{
				{ID: "n1", VisibilityStatus: "visible"},
				{ID: "n2", VisibilityStatus: "hidden"},
				{ID: "n3", VisibilityStatus: "prominent"},
			}
		}).Return(nil).Once()

		notes, err := repo.GetVisibleCommunityNotes(ctx, "obj")
		require.NoError(t, err)
		assert.Len(t, notes, 2)

		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "OBJECT#none#NOTES").Return(mockQuery).Once()
		mockQuery.On("Limit", 50).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		notes, err = repo.GetVisibleCommunityNotes(ctx, "none")
		require.NoError(t, err)
		assert.Empty(t, notes)
	})

	t.Run("update_score_and_analysis", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestCommunityNoteRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		// GetCommunityNote call
		mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.CommunityNote)
			*dest = models.CommunityNote{ID: "n1", ObjectID: "obj", ObjectType: "status", AuthorID: "u1", Content: "c", CreatedAt: time.Now()}
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		require.NoError(t, repo.UpdateCommunityNoteScore(ctx, "n1", 0.8, "visible"))

		// Analysis update path
		mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.CommunityNote)
			*dest = models.CommunityNote{ID: "n1", ObjectID: "obj", ObjectType: "status", AuthorID: "u1", Content: "c", CreatedAt: time.Now()}
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		require.NoError(t, repo.UpdateCommunityNoteAnalysis(ctx, "n1", 0.1, 0.9, 0.5))
	})
}

func TestCommunityNoteRepository_round09_vote_paths_and_author_listing(t *testing.T) {
	ctx := context.Background()

	t.Run("create_vote_and_user_votes_map", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestCommunityNoteRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil).Once()
		require.NoError(t, repo.CreateCommunityNoteVote(ctx, &storage.CommunityNoteVote{NoteID: "n1", VoterID: "u1", VoteType: "helpful", Weight: 1}))

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		assert.Error(t, repo.CreateCommunityNoteVote(ctx, &storage.CommunityNoteVote{NoteID: "n2", VoterID: "u1", VoteType: "helpful", Weight: 1}))

		// user votes over note IDs: first not found, second found, third error ignored
		mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "VOTE#u1").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		mockQuery.On("Where", "PK", "=", "NOTE#n2").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "VOTE#u1").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.CommunityNoteVote)
			*dest = models.CommunityNoteVote{NoteID: "n2", VoterID: "u1", VoteType: "helpful", Weight: 1.0, CreatedAt: time.Now()}
		}).Return(nil).Once()

		mockQuery.On("Where", "PK", "=", "NOTE#n3").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "VOTE#u1").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()

		votes, err := repo.GetUserCommunityNoteVotes(ctx, "u1", []string{"n1", "n2", "n3"})
		require.NoError(t, err)
		assert.Len(t, votes, 1)
	})

	t.Run("author_listing_and_note_votes", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestCommunityNoteRepo(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi3PK", "=", "AUTHOR#u1#NOTES").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("Where", "gsi3SK", "<", "123#n1").Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.CommunityNote)
			*dest = []models.CommunityNote{{ID: "n1", GSI3SK: "123#n1"}, {ID: "n2", GSI3SK: "122#n2"}}
		}).Return(nil).Once()
		notes, next, err := repo.GetCommunityNotesByAuthor(ctx, "u1", 2, "123#n1")
		require.NoError(t, err)
		assert.Len(t, notes, 2)
		assert.Equal(t, "122#n2", next)

		mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi3PK", "=", "AUTHOR#u1#NOTES").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		notes, next, err = repo.GetCommunityNotesByAuthor(ctx, "u1", 2, "badcursor")
		require.NoError(t, err)
		assert.Empty(t, notes)
		assert.Empty(t, next)

		// Votes on a note
		mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", "VOTE#").Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.CommunityNoteVote)
			*dest = []models.CommunityNoteVote{
				{NoteID: "n1", VoterID: "u1", VoteType: "helpful"},
				{NoteID: "n1", VoterID: "u2", VoteType: "not_helpful"},
			}
		}).Return(nil).Once()
		v, err := repo.GetCommunityNoteVotes(ctx, "n1")
		require.NoError(t, err)
		assert.Len(t, v, 2)
		assert.True(t, v[0].Helpful)

		mockQuery.On("Where", "PK", "=", "NOTE#none").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", "VOTE#").Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		v, err = repo.GetCommunityNoteVotes(ctx, "none")
		require.NoError(t, err)
		assert.Empty(t, v)
	})
}

