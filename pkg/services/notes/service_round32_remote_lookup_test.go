package notes

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round32StatusRepoStub struct {
	getStatusFunc      func(context.Context, string) (*models.Status, error)
	getStatusByURLFunc func(context.Context, string) (*models.Status, error)
}

func (s *round32StatusRepoStub) CreateStatus(context.Context, *models.Status) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) CreateBoostStatus(context.Context, *models.Status) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	if s.getStatusFunc != nil {
		return s.getStatusFunc(ctx, statusID)
	}
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetStatusByURL(ctx context.Context, url string) (*models.Status, error) {
	if s.getStatusByURLFunc != nil {
		return s.getStatusByURLFunc(ctx, url)
	}
	panic("unexpected call")
}

func (s *round32StatusRepoStub) UpdateStatus(context.Context, *models.Status) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) DeleteStatus(context.Context, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) DeleteBoostStatus(context.Context, string, string) (*models.Status, error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetPublicTimeline(context.Context, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetHomeTimeline(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetUserTimeline(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetConversationThread(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetConversationThreadReverse(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetReplies(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) SearchStatuses(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetStatusesByHashtag(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetTrendingStatuses(context.Context, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) LikeStatus(context.Context, string, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) UnlikeStatus(context.Context, string, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) ReblogStatus(context.Context, string, string, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) UnreblogStatus(context.Context, string, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) BookmarkStatus(context.Context, string, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) UnbookmarkStatus(context.Context, string, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) FlagStatus(context.Context, string, string, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) UnflagStatus(context.Context, string) error {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetFlaggedStatuses(context.Context, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetStatusesByIDs(context.Context, []string) ([]*models.Status, error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetStatusCounts(context.Context, string) (int, int, int, error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetStatusContext(context.Context, string) ([]*models.Status, []*models.Status, error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) GetStatusEngagement(context.Context, string, string) (bool, bool, bool, error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) CountStatusesByAuthor(context.Context, string) (int, error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) CountReplies(context.Context, string) (int, error) {
	panic("unexpected call")
}

func (s *round32StatusRepoStub) ListStatusesForAdmin(context.Context, *interfaces.StatusFilter, int, string) ([]*models.Status, string, error) {
	panic("unexpected call")
}

func TestResolveStatusForRead_UsesURLIndexBeforeCanonicalCandidates(t *testing.T) {
	remoteURL := "https://remote.example/users/bob/statuses/abc-123"
	expected := &models.Status{StatusID: models.CanonicalStatusIDForDomain(remoteURL, "example.com")}
	repo := &round32StatusRepoStub{
		getStatusByURLFunc: func(_ context.Context, url string) (*models.Status, error) {
			require.Equal(t, remoteURL, url)
			return expected, nil
		},
		getStatusFunc: func(context.Context, string) (*models.Status, error) {
			t.Fatal("GetStatus should not run when URL index resolves the note")
			return nil, nil
		},
	}

	service := &Service{domainName: "example.com", noteRepo: repo}
	status, err := service.resolveStatusForRead(context.Background(), remoteURL)
	require.NoError(t, err)
	require.Same(t, expected, status)
}

func TestResolveStatusForRead_FallsBackToCanonicalCandidates(t *testing.T) {
	t.Run("remote url candidates", func(t *testing.T) {
		remoteURL := "https://remote.example/users/bob/statuses/abc-123"
		canonicalID := models.CanonicalStatusIDForDomain(remoteURL, "example.com")
		var calls []string
		repo := &round32StatusRepoStub{
			getStatusByURLFunc: func(context.Context, string) (*models.Status, error) {
				return nil, storage.ErrNotFound
			},
			getStatusFunc: func(_ context.Context, statusID string) (*models.Status, error) {
				calls = append(calls, statusID)
				if statusID == "abc-123" {
					return &models.Status{StatusID: statusID}, nil
				}
				return nil, storage.ErrNotFound
			},
		}

		service := &Service{domainName: "example.com", noteRepo: repo}
		status, err := service.resolveStatusForRead(context.Background(), remoteURL)
		require.NoError(t, err)
		require.Equal(t, "abc-123", status.StatusID)
		require.Equal(t, []string{canonicalID, "abc-123"}, calls)
	})

	t.Run("local url candidates prefer path id", func(t *testing.T) {
		localURL := "https://example.com/users/alice/statuses/123"
		var calls []string
		repo := &round32StatusRepoStub{
			getStatusByURLFunc: func(context.Context, string) (*models.Status, error) {
				return nil, storage.ErrNotFound
			},
			getStatusFunc: func(_ context.Context, statusID string) (*models.Status, error) {
				calls = append(calls, statusID)
				return &models.Status{StatusID: statusID}, nil
			},
		}

		service := &Service{domainName: "example.com", noteRepo: repo}
		status, err := service.resolveStatusForRead(context.Background(), localURL)
		require.NoError(t, err)
		require.Equal(t, "123", status.StatusID)
		require.Equal(t, []string{"123"}, calls)
	})
}

func TestResolveStatusForRead_PropagatesErrorsAndGetNoteMapsNotFound(t *testing.T) {
	service := &Service{}
	_, err := service.resolveStatusForRead(context.Background(), "status-1")
	require.ErrorIs(t, err, ErrStatusRepositoryUnavailable)

	service = &Service{
		domainName: "example.com",
		noteRepo: &round32StatusRepoStub{
			getStatusFunc: func(context.Context, string) (*models.Status, error) {
				return nil, fmt.Errorf("boom")
			},
		},
	}
	_, err = service.resolveStatusForRead(context.Background(), "status-1")
	require.EqualError(t, err, "boom")

	remoteURL := "https://remote.example/users/bob/statuses/missing"
	service = &Service{
		domainName: "example.com",
		logger:     zap.NewNop(),
		noteRepo: &round32StatusRepoStub{
			getStatusByURLFunc: func(context.Context, string) (*models.Status, error) {
				return nil, storage.ErrNotFound
			},
			getStatusFunc: func(context.Context, string) (*models.Status, error) {
				return nil, storage.ErrNotFound
			},
		},
	}

	_, err = service.GetNote(context.Background(), remoteURL)
	require.ErrorIs(t, err, ErrStatusNotFound)
	require.True(t, statusLookupNotFound(storage.ErrNotFound))
	require.True(t, statusLookupNotFound(fmt.Errorf("wrapped not found")))
	require.False(t, statusLookupNotFound(fmt.Errorf("boom")))
}

func TestResolveStatusForRead_EmptyInputAndNonNotFoundErrors(t *testing.T) {
	service := &Service{
		domainName: "example.com",
		noteRepo:   &round32StatusRepoStub{},
	}

	_, err := service.resolveStatusForRead(context.Background(), "   ")
	require.ErrorIs(t, err, storage.ErrNotFound)

	remoteURL := "https://remote.example/users/bob/statuses/abc-123"
	service.noteRepo = &round32StatusRepoStub{
		getStatusByURLFunc: func(context.Context, string) (*models.Status, error) {
			return nil, fmt.Errorf("url lookup failed")
		},
		getStatusFunc: func(context.Context, string) (*models.Status, error) {
			t.Fatal("GetStatus should not run after a hard URL lookup failure")
			return nil, nil
		},
	}

	_, err = service.resolveStatusForRead(context.Background(), remoteURL)
	require.EqualError(t, err, "url lookup failed")

	service.noteRepo = &round32StatusRepoStub{
		getStatusFunc: func(context.Context, string) (*models.Status, error) {
			return nil, fmt.Errorf("candidate lookup failed")
		},
	}

	_, err = service.resolveStatusForRead(context.Background(), "status-1")
	require.EqualError(t, err, "candidate lookup failed")
}
