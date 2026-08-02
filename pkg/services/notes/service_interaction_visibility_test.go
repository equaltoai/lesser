package notes

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type statusInteractionVisibilityResult struct {
	status *models.Status
	events int
}

type statusInteractionVisibilityAction struct {
	name      string
	federates bool
	publishes bool
	invoke    func(context.Context, *Service, string, string) (statusInteractionVisibilityResult, error)
}

func TestStatusInteractionsCollapseHiddenAndDeletedStatusesToNotFound(t *testing.T) {
	for _, action := range statusInteractionVisibilityActions() {
		action := action
		t.Run(action.name, func(t *testing.T) {
			t.Run("followers-only non-follower", func(t *testing.T) {
				service, publisher, federation, _, _ := newNotesServiceHarness(t)
				service.noteRepo = fixedInteractionStatusRepository(service.noteRepo, protectedInteractionStatus(false))
				relationships := testingmocks.NewMockRelationshipRepository()
				service.relationshipRepo = relationships
				relationships.On("IsFollowing", mock.Anything, "mallory", "alice").Return(false, nil).Once()

				result, err := action.invoke(context.Background(), service, "protected-status", "mallory")

				require.ErrorIs(t, err, ErrStatusNotFound)
				require.Nil(t, result.status, "visibility denial must not return protected content")
				require.Zero(t, result.events)
				require.Empty(t, publisher.userEvents, "visibility denial must not emit interaction events")
				require.Empty(t, federation.activities, "visibility denial must not federate an interaction")
				relationships.AssertExpectations(t)
			})

			t.Run("deleted", func(t *testing.T) {
				service, publisher, federation, _, _ := newNotesServiceHarness(t)
				service.noteRepo = fixedInteractionStatusRepository(service.noteRepo, protectedInteractionStatus(true))

				result, err := action.invoke(context.Background(), service, "protected-status", "mallory")

				require.ErrorIs(t, err, ErrStatusNotFound)
				require.Nil(t, result.status, "deleted denial must not return status content")
				require.Zero(t, result.events)
				require.Empty(t, publisher.userEvents, "deleted denial must not emit interaction events")
				require.Empty(t, federation.activities, "deleted denial must not federate an interaction")
			})
		})
	}
}

func TestStatusInteractionsStillSucceedForAuthorizedViewer(t *testing.T) {
	for _, action := range statusInteractionVisibilityActions() {
		action := action
		t.Run(action.name, func(t *testing.T) {
			service, publisher, federation, _, _ := newNotesServiceHarness(t)
			service.noteRepo = fixedInteractionStatusRepository(service.noteRepo, protectedInteractionStatus(false))
			relationships := testingmocks.NewMockRelationshipRepository()
			service.relationshipRepo = relationships
			relationships.On("IsFollowing", mock.Anything, "bob", "alice").Return(true, nil).Once()

			result, err := action.invoke(context.Background(), service, "protected-status", "bob")

			require.NoError(t, err)
			require.NotNil(t, result.status)
			require.Equal(t, "protected content", result.status.Content)
			require.NotZero(t, result.events, "successful interaction must emit its user-visible effect")
			if action.publishes {
				require.NotEmpty(t, publisher.userEvents)
			} else {
				require.Empty(t, publisher.userEvents)
			}
			if action.federates {
				require.NotEmpty(t, federation.activities, "federated interaction must queue its activity")
			} else {
				require.Empty(t, federation.activities)
			}
			relationships.AssertExpectations(t)
		})
	}
}

func statusInteractionVisibilityActions() []statusInteractionVisibilityAction {
	return []statusInteractionVisibilityAction{
		{
			name:      "favourite (like)",
			federates: true,
			publishes: true,
			invoke: func(ctx context.Context, service *Service, statusID, actorID string) (statusInteractionVisibilityResult, error) {
				result, err := service.LikeNote(ctx, &LikeNoteCommand{StatusID: statusID, LikerID: actorID})
				if result == nil {
					return statusInteractionVisibilityResult{}, err
				}
				return statusInteractionVisibilityResult{status: result.Status, events: len(result.Events)}, err
			},
		},
		{
			name:      "unfavourite (unlike)",
			publishes: true,
			invoke: func(ctx context.Context, service *Service, statusID, actorID string) (statusInteractionVisibilityResult, error) {
				result, err := service.UnlikeNote(ctx, &UnlikeNoteCommand{StatusID: statusID, UnlikerID: actorID})
				if result == nil {
					return statusInteractionVisibilityResult{}, err
				}
				return statusInteractionVisibilityResult{status: result.Status, events: len(result.Events)}, err
			},
		},
		{
			name:      "bookmark",
			publishes: true,
			invoke: func(ctx context.Context, service *Service, statusID, actorID string) (statusInteractionVisibilityResult, error) {
				result, err := service.BookmarkNote(ctx, &BookmarkNoteCommand{StatusID: statusID, BookmarkerID: actorID})
				if result == nil {
					return statusInteractionVisibilityResult{}, err
				}
				return statusInteractionVisibilityResult{status: result.Status, events: len(result.Events)}, err
			},
		},
		{
			name:      "unbookmark",
			publishes: true,
			invoke: func(ctx context.Context, service *Service, statusID, actorID string) (statusInteractionVisibilityResult, error) {
				result, err := service.UnbookmarkNote(ctx, &UnbookmarkNoteCommand{StatusID: statusID, UnbookmarkerID: actorID})
				if result == nil {
					return statusInteractionVisibilityResult{}, err
				}
				return statusInteractionVisibilityResult{status: result.Status, events: len(result.Events)}, err
			},
		},
		{
			name: "mute",
			invoke: func(ctx context.Context, service *Service, statusID, actorID string) (statusInteractionVisibilityResult, error) {
				result, err := service.MuteNote(ctx, &MuteNoteCommand{StatusID: statusID, MuterID: actorID})
				if result == nil {
					return statusInteractionVisibilityResult{}, err
				}
				return statusInteractionVisibilityResult{status: result.Status, events: len(result.Events)}, err
			},
		},
		{
			name: "unmute",
			invoke: func(ctx context.Context, service *Service, statusID, actorID string) (statusInteractionVisibilityResult, error) {
				result, err := service.UnmuteNote(ctx, &UnmuteNoteCommand{StatusID: statusID, MuterID: actorID})
				if result == nil {
					return statusInteractionVisibilityResult{}, err
				}
				return statusInteractionVisibilityResult{status: result.Status, events: len(result.Events)}, err
			},
		},
		{
			name:      "reblog",
			federates: true,
			publishes: true,
			invoke: func(ctx context.Context, service *Service, statusID, actorID string) (statusInteractionVisibilityResult, error) {
				result, err := service.ReblogNote(ctx, &ReblogNoteCommand{StatusID: statusID, RebloggerID: actorID})
				if result == nil {
					return statusInteractionVisibilityResult{}, err
				}
				return statusInteractionVisibilityResult{status: result.Status, events: len(result.Events)}, err
			},
		},
		{
			name:      "unreblog",
			publishes: true,
			invoke: func(ctx context.Context, service *Service, statusID, actorID string) (statusInteractionVisibilityResult, error) {
				result, err := service.UnreblogNote(ctx, &UnreblogNoteCommand{StatusID: statusID, UnrebloggerID: actorID})
				if result == nil {
					return statusInteractionVisibilityResult{}, err
				}
				return statusInteractionVisibilityResult{status: result.Status, events: len(result.Events)}, err
			},
		},
	}
}

func fixedInteractionStatusRepository(repo interfaces.StatusRepository, status *models.Status) interfaces.StatusRepository {
	return &delegatingStatusRepo{
		StatusRepository: repo,
		getStatus: func(context.Context, string) (*models.Status, error) {
			clone := *status
			return &clone, nil
		},
	}
}

func protectedInteractionStatus(deleted bool) *models.Status {
	return &models.Status{
		StatusID:       "protected-status",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Content:        "protected content",
		Visibility:     models.VisibilityPrivate,
		Deleted:        deleted,
	}
}
