package notes

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubCommunityNoteRepo struct {
	createNoteErr error
	createdNote   *storage.CommunityNote

	getNoteErr error
	getNote    *storage.CommunityNote

	visibleErr   error
	visibleNotes []*storage.CommunityNote

	createVoteErr error
	createdVote   *storage.CommunityNoteVote

	getVotesErr error
	votes       []*storage.CommunityNoteVote

	userVotesErr error
	userVotes    map[string]*storage.CommunityNoteVote

	notesByAuthorErr error
	notesByAuthor    []*storage.CommunityNote

	updateScoreErr error
	updatedScore   struct {
		noteID string
		score  float64
		status string
	}
}

func (s *stubCommunityNoteRepo) CreateCommunityNote(_ context.Context, note *storage.CommunityNote) error {
	s.createdNote = note
	return s.createNoteErr
}

func (s *stubCommunityNoteRepo) GetCommunityNote(_ context.Context, _ string) (*storage.CommunityNote, error) {
	return s.getNote, s.getNoteErr
}

func (s *stubCommunityNoteRepo) GetVisibleCommunityNotes(_ context.Context, _ string) ([]*storage.CommunityNote, error) {
	return s.visibleNotes, s.visibleErr
}

func (s *stubCommunityNoteRepo) CreateCommunityNoteVote(_ context.Context, vote *storage.CommunityNoteVote) error {
	s.createdVote = vote
	return s.createVoteErr
}

func (s *stubCommunityNoteRepo) GetCommunityNoteVotes(_ context.Context, _ string) ([]*storage.CommunityNoteVote, error) {
	return s.votes, s.getVotesErr
}

func (s *stubCommunityNoteRepo) GetUserCommunityNoteVotes(_ context.Context, _ string, _ []string) (map[string]*storage.CommunityNoteVote, error) {
	return s.userVotes, s.userVotesErr
}

func (s *stubCommunityNoteRepo) GetCommunityNotesByAuthor(_ context.Context, _ string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	if s.notesByAuthorErr != nil {
		return nil, "", s.notesByAuthorErr
	}
	start := 0
	if cursor != "" {
		parsed, err := strconv.Atoi(cursor)
		if err == nil && parsed >= 0 {
			start = parsed
		}
	}
	if start >= len(s.notesByAuthor) {
		return []*storage.CommunityNote{}, "", nil
	}
	end := len(s.notesByAuthor)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	nextCursor := ""
	if end < len(s.notesByAuthor) {
		nextCursor = strconv.Itoa(end)
	}
	return s.notesByAuthor[start:end], nextCursor, nil
}

func (s *stubCommunityNoteRepo) UpdateCommunityNoteScore(_ context.Context, noteID string, score float64, status string) error {
	s.updatedScore.noteID = noteID
	s.updatedScore.score = score
	s.updatedScore.status = status
	return s.updateScoreErr
}

func TestService_CreateNote_StoresDefaults(t *testing.T) {
	repo := &stubCommunityNoteRepo{}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	note := &CommunityNote{
		ObjectID:   "obj1",
		ObjectType: "Note",
		AuthorID:   "alice",
		Content:    "hello",
		Language:   "en",
		Sources:    []Source{{URL: "https://example.com"}},
	}

	err := svc.CreateNote(context.Background(), note)
	require.NoError(t, err)
	require.NotNil(t, repo.createdNote)

	assert.NotEmpty(t, note.ID)
	assert.True(t, note.CreatedAt.After(time.Time{}))
	assert.Equal(t, VisibilityPending, note.VisibilityStatus)
	assert.Equal(t, 0.0, note.Score)
	assert.True(t, note.UpdatedAt.After(time.Time{}))
	assert.Equal(t, note.ID, repo.createdNote.ID)
}

func TestService_CheckNoteRateLimit_PaginatesPastOldNotes(t *testing.T) {
	now := time.Now()
	oldNotes := make([]*storage.CommunityNote, communityNoteRateLimitPageSize)
	for i := range oldNotes {
		oldNotes[i] = &storage.CommunityNote{ID: strconv.Itoa(i), CreatedAt: now.Add(-48 * time.Hour)}
	}
	recentNotes := []*storage.CommunityNote{
		{ID: "recent-1", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "recent-2", CreatedAt: now.Add(-1 * time.Hour)},
	}
	repo := &stubCommunityNoteRepo{
		notesByAuthor: append(oldNotes, recentNotes...),
	}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	allowed, remaining := svc.CheckNoteRateLimit(context.Background(), "alice", 2)
	assert.False(t, allowed)
	assert.Equal(t, 0, remaining)
}

func TestService_StoreNote_ErrorWrapped(t *testing.T) {
	repo := &stubCommunityNoteRepo{createNoteErr: errors.New("boom")}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	err := svc.StoreNote(context.Background(), &CommunityNote{ID: "n1", Sources: []Source{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to store note")
}

func TestService_GetNote_ConvertsVisibility(t *testing.T) {
	repo := &stubCommunityNoteRepo{
		getNote: &storage.CommunityNote{
			ID:               "n1",
			ObjectID:         "obj",
			AuthorID:         "alice",
			Content:          "x",
			VisibilityStatus: "visible",
			Sources:          []string{"https://example.com"},
		},
	}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	note, err := svc.GetNote(context.Background(), "n1")
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.Equal(t, VisibilityVisible, note.VisibilityStatus)
	require.Len(t, note.Sources, 1)
	assert.Equal(t, "https://example.com", note.Sources[0].URL)
}

func TestService_GetVisibleNotes_ConvertsSlice(t *testing.T) {
	repo := &stubCommunityNoteRepo{
		visibleNotes: []*storage.CommunityNote{
			{ID: "n1", VisibilityStatus: "hidden"},
		},
	}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	notes, err := svc.GetVisibleNotes(context.Background(), "obj")
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, VisibilityHidden, notes[0].VisibilityStatus)
}

func TestService_VotesAndUserVotes(t *testing.T) {
	repo := &stubCommunityNoteRepo{
		votes: []*storage.CommunityNoteVote{
			{NoteID: "n1", VoterID: "u1", VoteType: "helpful", Weight: 1},
			{NoteID: "n1", VoterID: "u2", VoteType: "not_helpful", Weight: 1},
		},
		userVotes: map[string]*storage.CommunityNoteVote{
			"n1": {NoteID: "n1", VoterID: "u1", VoteType: "helpful", Weight: 1},
			"n2": nil,
		},
	}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	require.NoError(t, svc.StoreVote(context.Background(), &Vote{NoteID: "n1", VoterID: "u1", VoteType: VoteHelpful, Weight: 1}))
	require.NotNil(t, repo.createdVote)

	votes, err := svc.GetVotesForNote(context.Background(), "n1")
	require.NoError(t, err)
	require.Len(t, votes, 2)
	assert.Equal(t, VoteHelpful, votes[0].VoteType)
	assert.Equal(t, VoteNotHelpful, votes[1].VoteType)

	userVotes, err := svc.GetUserVotes(context.Background(), "u1", []string{"n1", "n2"})
	require.NoError(t, err)
	require.Len(t, userVotes, 1)
	assert.Equal(t, VoteHelpful, userVotes["n1"].VoteType)
}

func TestService_GetNotesByAuthor_Converts(t *testing.T) {
	repo := &stubCommunityNoteRepo{
		notesByAuthor: []*storage.CommunityNote{{ID: "n1", AuthorID: "alice"}},
	}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	notes, err := svc.GetNotesByAuthor(context.Background(), "alice", 10)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "n1", notes[0].ID)
}

func TestService_RecalculateNoteScore_UpdatesStorage(t *testing.T) {
	now := time.Now()
	var votes []*storage.CommunityNoteVote
	for i := 0; i < 10; i++ {
		votes = append(votes, &storage.CommunityNoteVote{NoteID: "n1", VoterID: "u", VoteType: "helpful", Weight: 1, CreatedAt: now})
	}
	repo := &stubCommunityNoteRepo{
		getNote: &storage.CommunityNote{
			ID:               "n1",
			AuthorID:         "alice",
			VisibilityStatus: "pending",
			Sentiment:        1,
			Objectivity:      1,
			SourceQuality:    1,
			CreatedAt:        now,
		},
		votes: votes,
	}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	err := svc.RecalculateNoteScore(context.Background(), "n1")
	require.NoError(t, err)
	assert.Equal(t, "n1", repo.updatedScore.noteID)
	assert.Equal(t, string(VisibilityVisible), repo.updatedScore.status)
	assert.Greater(t, repo.updatedScore.score, 0.0)
}

func TestService_CheckNoteRateLimit_CountsRecentNotes(t *testing.T) {
	now := time.Now()
	repo := &stubCommunityNoteRepo{
		notesByAuthor: []*storage.CommunityNote{
			{ID: "n1", CreatedAt: now.Add(-2 * time.Hour)},
			{ID: "n2", CreatedAt: now.Add(-48 * time.Hour)},
		},
	}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	allowed, remaining := svc.CheckNoteRateLimit(context.Background(), "alice", 2)
	assert.True(t, allowed)
	assert.Equal(t, 1, remaining)
}

// TestService_CheckRateLimit_DeniesWhenAtLimit verifies CSR-048:
// when the author already has 5 notes in the last 24 hours and reputation
// 500 grants a limit of 5, the next creation is denied.
func TestService_CheckRateLimit_DeniesWhenAtLimit(t *testing.T) {
	now := time.Now()
	notes := make([]*storage.CommunityNote, 5)
	for i := range notes {
		notes[i] = &storage.CommunityNote{
			ID:        "recent-" + strconv.Itoa(i),
			CreatedAt: now.Add(-time.Duration(i) * time.Hour),
		}
	}
	repo := &stubCommunityNoteRepo{notesByAuthor: notes}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	allowed, remaining := svc.CheckRateLimit(context.Background(), "alice", 500.0)
	assert.False(t, allowed)
	assert.Equal(t, 0, remaining)
}

// TestService_CheckRateLimit_AllowsBelowLimit verifies CSR-048:
// when the author has created fewer notes than the limit grants,
// creation is allowed.
func TestService_CheckRateLimit_AllowsBelowLimit(t *testing.T) {
	now := time.Now()
	notes := make([]*storage.CommunityNote, 4)
	for i := range notes {
		notes[i] = &storage.CommunityNote{
			ID:        "recent-" + strconv.Itoa(i),
			CreatedAt: now.Add(-time.Duration(i) * time.Hour),
		}
	}
	repo := &stubCommunityNoteRepo{notesByAuthor: notes}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	allowed, remaining := svc.CheckRateLimit(context.Background(), "alice", 500.0)
	assert.True(t, allowed)
	assert.Equal(t, 1, remaining)
}

// TestService_CheckRateLimit_DeniesOnLowReputation verifies CSR-048:
// users with reputation below MinReputationToCreateNotes cannot create notes.
func TestService_CheckRateLimit_DeniesOnLowReputation(t *testing.T) {
	repo := &stubCommunityNoteRepo{}
	svc := &Service{repo: repo, logger: zap.NewNop()}

	allowed, remaining := svc.CheckRateLimit(context.Background(), "alice", MinReputationToCreateNotes-1)
	assert.False(t, allowed)
	assert.Equal(t, 0, remaining)
}
